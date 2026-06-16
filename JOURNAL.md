# Project Journal

## 2026-06-14
- Added "Generate using AI" button to the deck creation process.
- Implemented an intermediate "AI Configuration" form that allows users to define parameters like target language and quantity.
- Integrated Ollama as a backend for LLM generation, allowing local usage via even-less logic.
- Built a robust multi-step flow: 1) Input selection -> 2) Transition with CSS animations/spinners during processing -> 3. auto-generation of deck cards -> 4. Success state.
- Added dynamic system prompt injection to ensure the LLM uses the selected target language in its response format.
- Implemented logic to preserve user input and display specific error messages when AI generation fails, enabling easy retry functionality without losing context.
