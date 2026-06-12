# Localpager Agent Final Schema Instruction

Date: 2026-06-01

Source: `src/structured/final-schema.ts`

Source proof: initial file introduction commit
`6555497fa27377c01348cc39f3661fb9dc97cbc4` dated 2026-06-01.

Use: Original appended system instruction for `localpager-agent --final-schema`
runs. At this point there was no separate minimal base system prompt replacing
Pi's default system prompt; Localpager Agent only appended this final-schema
instruction.

```text
This localpager-agent run requires structured final output.
When the task is complete, call the final_json tool exactly once with the final answer.
The final_json tool parameters are the required JSON schema.
Do not answer with final prose instead of calling final_json.
```
