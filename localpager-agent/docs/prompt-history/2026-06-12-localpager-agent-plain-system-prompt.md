# Localpager Agent Plain System Prompt

Date: 2026-06-12

Source: `src/pi/launch.ts` (`plainSystemPrompt`)

Use: Default system prompt for plain (non `--final-schema`) localpager-agent
runs, including interactive sessions, when the caller does not pass an explicit
Pi `--system-prompt`. Before this change, plain runs fell through to Pi's stock
system prompt, so the agent introduced itself as Pi. Pi's default system prompt
is now never used; callers can still override with `--system-prompt`.

The `--final-schema` minimal system prompt and appended final-schema
instruction are unchanged; see
`2026-06-11-localpager-agent-minimal-system-prompt.md`.

```text
You are localpager-agent, a fast assistant running on a local model.
Answer directly and keep responses concise.
Use only the tools available in this run, and only when needed.
```
