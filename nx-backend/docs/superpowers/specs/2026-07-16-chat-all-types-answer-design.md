# Chat All-Types Answer Design

## Problem

When a user asks how to apply the Enneagram to children across types 1 through 9, the chat can answer only for the user's own primary type (for example type 6). Two current behaviors combine to cause this: retrieval boosts the user's primary type, and the final prompt always asks for a 1-3 sentence answer with a small token budget.

## Desired behavior

Questions that explicitly request all nine types, types 1-9, every type, or a type-by-type comparison must receive a complete answer covering types 1 through 9 in numeric order. Each type should include the child's typical characteristics, how a parent can understand or communicate with the child, and one concrete application. The user's own primary type must not narrow the requested scope.

Ordinary single-type questions should remain concise.

## Design

Add a focused breadth detector in the LLM prompt builder. It recognizes explicit all-type wording and changes only two model-facing controls:

1. Append an all-types coverage instruction instead of the fixed concise instruction. The instruction requires numbered coverage from 1 through 9 and names the three requested content elements.
2. Allocate a larger token budget for all-types questions so the model has enough room to comply.

Retrieval remains advisory. The prompt explicitly states that retrieved documents may be incomplete and that the model should use safe general Enneagram knowledge to fill missing types. This avoids coupling answer completeness to the current top-four retrieval limit while preserving sources and normal retrieval behavior.

## Testing

Add unit tests proving that a representative children question triggers the all-types instruction and expanded budget, while a normal single-type question retains the concise instruction and small budget. Run the focused LLM tests, then the full server test suite.
