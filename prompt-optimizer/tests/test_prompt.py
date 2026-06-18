from __future__ import annotations

import unittest

from prompt_optimizer.prompt import (
    ROUTING_POLICY_OVERLAY_PLACEHOLDER,
    PromptError,
    load_overlay_seed_prompt,
    normalize_template_variables,
    split_overlay_seed_prompt,
    split_seed_prompt,
    validate_placeholders,
)


SAMPLE_PROMPT = """# Classifier

Allowed:
{{{allowed_topics_json}}}

Descriptions:
{{{topic_descriptions}}}

## Goal

Choose labels.

## Decision Process

Be careful.

## Target

`{{target}}`

## GitHub Context

{{{github_context}}}
"""


class PromptTest(unittest.TestCase):
    def test_normalizes_handlebars_variables(self) -> None:
        normalized = normalize_template_variables(SAMPLE_PROMPT)

        self.assertIn("__ALLOWED_TOPICS_JSON__", normalized)
        self.assertIn("__TOPIC_DESCRIPTIONS__", normalized)
        self.assertIn("__TARGET__", normalized)
        self.assertIn("__GITHUB_CONTEXT__", normalized)
        self.assertNotIn("{{target}}", normalized)

    def test_extracts_routing_policy_and_preserves_scaffold(self) -> None:
        parts = split_seed_prompt(normalize_template_variables(SAMPLE_PROMPT))

        self.assertIn("## Goal", parts.routing_policy)
        self.assertIn("## Decision Process", parts.routing_policy)
        self.assertNotIn("## Target", parts.routing_policy)
        self.assertIn("__ALLOWED_TOPICS_JSON__", parts.prefix)
        self.assertIn("__TARGET__", parts.suffix)
        self.assertEqual(parts.assemble(parts.routing_policy), parts.template)

    def test_extracts_v10_inner_monologue_policy_block(self) -> None:
        prompt = """# Classifier

{{{allowed_topics_json}}}
{{{topic_descriptions}}}

## Inner Monologue

Keep reasoning short.

## Evalstate Boundary Guidance

Use the smallest correct topic set.

## Target

`{{target}}`

## GitHub Context

{{{github_context}}}
"""

        parts = split_seed_prompt(normalize_template_variables(prompt))

        self.assertIn("## Inner Monologue", parts.routing_policy)
        self.assertIn("## Evalstate Boundary Guidance", parts.routing_policy)
        self.assertNotIn("## Target", parts.routing_policy)
        self.assertIn("__ALLOWED_TOPICS_JSON__", parts.prefix)
        self.assertIn("__TARGET__", parts.suffix)
        self.assertEqual(parts.assemble(parts.routing_policy), parts.template)

    def test_extracts_overlay_only_seed_policy(self) -> None:
        scaffold = normalize_template_variables(
            """# Classifier

{{{allowed_topics_json}}}
{{{topic_descriptions}}}

## Evalstate Boundary Guidance

Fixed definitions stay here.

## Routing Policy Overlay

{{{routing_policy_overlay}}}

## Target

`{{target}}`

## GitHub Context

{{{github_context}}}
"""
        )
        overlay = """# Decision Procedure

Use central surfaces.
"""

        parts = split_overlay_seed_prompt(scaffold, overlay)

        self.assertIn("Fixed definitions stay here.", parts.prefix)
        self.assertEqual(parts.routing_policy, overlay)
        self.assertNotIn("Fixed definitions stay here.", parts.routing_policy)
        self.assertNotIn("## Target", parts.routing_policy)
        self.assertIn("__TARGET__", parts.suffix)
        self.assertNotIn(ROUTING_POLICY_OVERLAY_PLACEHOLDER, parts.assemble(parts.routing_policy))

    def test_loads_committed_evalstate_overlay_prompt(self) -> None:
        parts = load_overlay_seed_prompt()

        self.assertIn("## Evalstate Taxonomy", parts.prefix)
        self.assertIn("## Evalstate Boundary Guidance", parts.prefix)
        self.assertIn("## Localpager Output Mapping", parts.prefix)
        self.assertIn("# Decision Procedure", parts.routing_policy)
        self.assertIn("# Cardinality Rules", parts.routing_policy)
        self.assertIn("# Boundary Overlays", parts.routing_policy)
        self.assertIn("# Suppression Rules", parts.routing_policy)
        self.assertNotIn("Allowed topic IDs", parts.routing_policy)
        self.assertNotIn("__ALLOWED_TOPICS_JSON__", parts.routing_policy)
        self.assertNotIn("__TOPIC_DESCRIPTIONS__", parts.routing_policy)
        self.assertNotIn("## Target", parts.routing_policy)
        self.assertIn("__TARGET__", parts.suffix)
        self.assertEqual(parts.assemble(parts.routing_policy), parts.template)

    def test_rejects_scaffold_without_single_overlay_placeholder(self) -> None:
        scaffold = normalize_template_variables(SAMPLE_PROMPT)

        with self.assertRaisesRegex(PromptError, "exactly one routing policy overlay placeholder"):
            split_overlay_seed_prompt(scaffold, "# Decision Procedure\n")

    def test_rejects_missing_placeholders(self) -> None:
        with self.assertRaisesRegex(PromptError, "missing required placeholder"):
            validate_placeholders("__TARGET__ only")


if __name__ == "__main__":
    unittest.main()
