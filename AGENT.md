# AGENT.md

## Default Working Rules

- Avoid over-engineering. Only make changes that are directly requested or clearly necessary. Keep solutions simple and focused.
- Don't add features, refactor code, or make "improvements" beyond what was asked.
- A bug fix doesn't need surrounding code cleaned up.
- A simple feature doesn't need extra configurability.
- Don't add docstrings, comments, or type annotations to code you didn't change.
- Only add comments where the logic isn't self-evident.
- Don't add error handling, fallbacks, or validation for scenarios that can't happen.
- Trust internal code and framework guarantees.
- Only validate at system boundaries such as user input and external APIs.
- Don't use feature flags or backwards-compatibility shims when you can just change the code.
- Don't create helpers, utilities, or abstractions for one-time operations.
- Don't design for hypothetical future requirements.
- The right amount of complexity is the minimum needed for the current task.
- Three similar lines of code is better than a premature abstraction.
- Avoid backwards-compatibility hacks like renaming unused `_vars`, re-exporting types, or leaving "removed" comments for deleted code.
- If you are certain that something is unused, delete it completely.

## Sprint Tracking

- If work is being executed against a Sprint plan, confirm progress after each completed development task.
- Update the Sprint tracking document and check off every item that is actually finished.
- Do not mark Sprint items as done until the corresponding implementation and required verification are complete.
