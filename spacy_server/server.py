"""
Picobrain SpaCy Dependency Parsing Server.

Implements Algorithm 1 from arXiv:2507.03226:
  Dependency-Based Knowledge Graph Construction with Coreference.

Pipeline:
  1. SpaCy NLP parsing (tokenize, POS, dependency parse, NER)
  2. Dependency triple extraction (nsubj-ROOT-dobj, ROOT-prep-pobj)
  3. Linear extraction heuristics (copula, apposition, possessive)
  4. Passive voice normalization (flip active direction)
  5. Phrasal merging (compound nouns, named entities)
  6. Coreference resolution (map pronouns to canonical entities)
  7. Filtering (remove <2 chars, stopwords, punctuation)
  8. Normalization (lowercase, strip articles, deduplicate)
"""

import spacy
from fastapi import FastAPI
from pydantic import BaseModel
from typing import List, Optional
import uvicorn
import re

app = FastAPI()

# --- Globals ---
_nlp = None
_stopwords = {
    "the", "a", "an", "this", "that", "these", "those",
    "is", "are", "was", "were", "be", "been", "being",
    "have", "has", "had", "do", "does", "did",
    "will", "would", "could", "should", "may", "might",
    "shall", "can", "must", "need", "dare",
    "it", "its", "they", "them", "their", "we", "our",
    "i", "me", "my", "you", "your", "he", "him", "his",
    "she", "her", "hers",
    "and", "or", "but", "if", "then", "so", "as",
    "not", "no", "nor", "too", "very",
    "of", "in", "on", "at", "to", "for", "with",
    "about", "from", "by", "into", "through",
    "up", "down", "out", "off", "over", "under",
    "also", "just", "than", "then", "now",
    "here", "there", "when", "where", "how", "what", "which",
    "who", "whom", "whose",
}

# --- Models ---

class ParseRequest(BaseModel):
    text: str

class Triple(BaseModel):
    head: str
    relation: str
    tail: str

class ParseResponse(BaseModel):
    triples: List[Triple]

class HealthResponse(BaseModel):
    status: str
    model: str

# --- Pipeline Functions ---

def get_nlp():
    """Load the SpaCy model lazily."""
    global _nlp
    if _nlp is None:
        _nlp = spacy.load("en_core_web_sm")
        # Add coreference if available
        try:
            _nlp.add_pipe("coref", source=spacy.load("en_core_web_sm_coref"))
        except Exception:
            pass  # Coref is optional
    return _nlp


def merge_phrases(doc):
    """Merge compound nouns and named entities into single tokens."""
    # Merge named entities
    with doc.retokenize() as retokenizer:
        for ent in doc.ents:
            if len(ent) > 1:
                try:
                    retokenizer.merge(ent, attrs={"ENT_TYPE": ent.label_})
                except ValueError:
                    pass

    # Merge compound nouns (compound + head)
    # We collect spans to merge first, then merge
    spans_to_merge = []
    for token in doc:
        if token.dep_ == "compound" and token.head.i == token.i + 1:
            span = doc[token.i : token.head.i + 1]
            if len(span) > 1:
                spans_to_merge.append(span)

    with doc.retokenize() as retokenizer:
        for span in spans_to_merge:
            try:
                retokenizer.merge(span)
            except ValueError:
                pass

    return doc


def extract_dependency_triples(doc):
    """Extract (head, relation, tail) triples from the dependency tree.

    Patterns:
    - nsubj + ROOT + dobj: "SAP launched Joule" -> (SAP, launched, Joule)
    - ROOT + prep + pobj: "launched for Consultants" -> (launched, for, Consultants)
    - nsubj + ROOT + attr: "Joule is a tool" -> (Joule, is, tool)
    - nsubjpass + ROOT + agent: "launched by SAP" -> normalized to (SAP, launched, X)
    """
    triples = []

    for token in doc:
        # Pattern 1: nsubj - verb - dobj
        if token.dep_ in ("nsubj", "nsubjpass"):
            verb = token.head
            subject = token
            objects = [t for t in verb.children if t.dep_ in ("dobj", "attr", "pobj")]

            for obj in objects:
                rel = verb.lemma_.lower()
                head_ent = subject.text.strip()
                tail_ent = obj.text.strip()

                # Handle passive voice: "X was launched by Y" -> (Y, launched, X)
                if token.dep_ == "nsubjpass":
                    agent = [t for t in verb.children if t.dep_ == "agent"]
                    if agent:
                        head_ent = agent[0].text.strip()
                        tail_ent = subject.text.strip()
                    else:
                        # Passive without agent: "X was launched" -> skip or keep
                        head_ent = subject.text.strip()
                        tail_ent = obj.text.strip()

                triples.append((head_ent, rel, tail_ent))

            # If no direct object, check for prep phrases
            if not objects:
                for child in verb.children:
                    if child.dep_ == "prep":
                        pobjs = [t for t in child.children if t.dep_ == "pobj"]
                        for pobj in pobjs:
                            rel = f"{verb.lemma_.lower()}_{child.lemma_.lower()}"
                            triples.append((subject.text.strip(), rel, pobj.text.strip()))

        # Pattern 2: verb - prep - pobj (standalone prepositional)
        elif token.dep_ == "prep" and token.head.pos_ in ("VERB", "NOUN"):
            pobjs = [t for t in token.children if t.dep_ == "pobj"]
            for pobj in pobjs:
                rel = f"{token.head.lemma_.lower()}_{token.lemma_.lower()}" if token.head.pos_ == "VERB" else token.lemma_.lower()
                triples.append((token.head.text.strip(), rel, pobj.text.strip()))

    return triples


def extract_linear_triples(doc):
    """Extract triples missed by dependency parsing using linear heuristics.

    Patterns:
    - Copula: "X is Y" (where Y is not a verb) -> (X, is, Y)
    - Apposition: "X, a Y, ..." -> (X, is, Y)
    - Possessive: "X's Y" -> (X, has, Y)
    """
    triples = []

    for i, token in enumerate(doc):
        # Copula pattern: "X is Y"
        if token.dep_ == "attr" and token.head.lemma_ == "be":
            subject = [t for t in token.head.children if t.dep_ in ("nsubj", "nsubjpass")]
            for s in subject:
                triples.append((s.text.strip(), "is", token.text.strip()))

        # Possessive: "X's Y"
        if token.dep_ == "poss":
            head = token.head
            triples.append((token.text.strip(), "has", head.text.strip()))

        # Apposition: "X, a Y,"
        if token.dep_ == "appos":
            head = token.head
            triples.append((head.text.strip(), "is", token.text.strip()))

    return triples


def build_coref_map(doc):
    """Build a coreference resolution map.

    Uses SpaCy's coref extension if available, otherwise falls back
    to a simple pronoun-resolution heuristic.
    """
    coref_map = {}

    # Try SpaCy coref extension
    if hasattr(doc._, "coref_chains") and doc._.coref_chains:
        for chain in doc._.coref_chains:
            mentions = list(chain)
            if len(mentions) >= 2:
                # Use the first non-pronoun mention as canonical
                canonical = None
                for mention in mentions:
                    token = doc[mention[0]]
                    if token.pos_ != "PRON" and len(token.text.strip()) >= 2:
                        canonical = token.text.strip()
                        break
                if canonical:
                    for mention in mentions:
                        token = doc[mention[0]]
                        coref_map[token.text.strip().lower()] = canonical

    # Fallback: simple pronoun resolution via last-named-entity heuristic
    if not coref_map:
        last_entity = None
        for token in doc:
            if token.ent_type_:
                last_entity = token.text.strip()
            elif token.pos_ == "PRON" and last_entity:
                coref_map[token.text.strip().lower()] = last_entity

    return coref_map


def resolve_coref(triples, coref_map):
    """Replace pronouns and mentions with canonical entities."""
    resolved = []
    for head, rel, tail in triples:
        h = coref_map.get(head.lower(), head)
        t = coref_map.get(tail.lower(), tail)
        resolved.append((h, rel, t))
    return resolved


def normalize_passive(triples):
    """Normalize passive constructions to active direction.

    "X was launched by Y" should become (Y, launched, X).
    This is mostly handled in extract_dependency_triples, but we also
    check for passive relation markers here.
    """
    normalized = []
    for head, rel, tail in triples:
        # Clean up relation
        rel = rel.strip().lower()
        # Remove auxiliary markers
        rel = re.sub(r"^(was|were|is|are|been|being)\s+", "", rel)
        normalized.append((head, rel, tail))
    return normalized


def filter_triples(triples):
    """Remove invalid triples: short entities, stopwords, punctuation-only."""
    filtered = []
    seen = set()

    for head, rel, tail in triples:
        head = head.strip()
        rel = rel.strip()
        tail = tail.strip()

        # Skip empty or too-short entities
        if len(head) < 2 or len(tail) < 2 or len(rel) < 1:
            continue

        # Skip stopwords as entities
        if head.lower() in _stopwords or tail.lower() in _stopwords:
            continue

        # Skip punctuation-only
        if re.match(r"^[\W_]+$", head) or re.match(r"^[\W_]+$", tail):
            continue

        # Skip self-references
        if head.lower() == tail.lower():
            continue

        # Deduplicate
        key = (head.lower(), rel.lower(), tail.lower())
        if key in seen:
            continue
        seen.add(key)

        filtered.append((head, rel, tail))

    return filtered


def normalize_triples(triples):
    """Final normalization: strip articles, clean whitespace."""
    normalized = []
    for head, rel, tail in triples:
        # Strip leading articles
        head = re.sub(r"^(the|a|an)\s+", "", head, flags=re.IGNORECASE)
        tail = re.sub(r"^(the|a|an)\s+", "", tail, flags=re.IGNORECASE)

        # Clean whitespace
        head = " ".join(head.split())
        rel = " ".join(rel.split())
        tail = " ".join(tail.split())

        normalized.append((head, rel, tail))
    return normalized


def extract_triples(text):
    """Full pipeline: parse text and extract (head, relation, tail) triples.

    Implements Algorithm 1 from arXiv:2507.03226.
    """
    nlp = get_nlp()
    doc = nlp(text)

    # Step 1: Merge phrases (compound nouns, named entities)
    doc = merge_phrases(doc)

    # Step 2: Extract dependency triples
    dep_triples = extract_dependency_triples(doc)

    # Step 3: Extract linear triples
    lin_triples = extract_linear_triples(doc)

    # Step 4: Union both sets
    all_triples = dep_triples + lin_triples

    # Step 5: Build coreference map
    coref_map = build_coref_map(doc)

    # Step 6: Resolve coreferences
    all_triples = resolve_coref(all_triples, coref_map)

    # Step 7: Normalize passive voice
    all_triples = normalize_passive(all_triples)

    # Step 8: Filter
    all_triples = filter_triples(all_triples)

    # Step 9: Normalize
    all_triples = normalize_triples(all_triples)

    return all_triples


# --- API Endpoints ---

@app.post("/parse", response_model=ParseResponse)
def parse(req: ParseRequest):
    triples = extract_triples(req.text)
    return ParseResponse(
        triples=[Triple(head=h, relation=r, tail=t) for h, r, t in triples]
    )


@app.get("/health", response_model=HealthResponse)
def health():
    nlp = get_nlp()
    return HealthResponse(status="healthy", model=nlp.meta.get("name", "unknown"))


if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000)