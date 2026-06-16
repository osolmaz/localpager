from __future__ import annotations

import unittest

from prompt_optimizer.prompt import (
    PromptError,
    normalize_template_variables,
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

    def test_rejects_missing_placeholders(self) -> None:
        with self.assertRaisesRegex(PromptError, "missing required placeholder"):
            validate_placeholders("__TARGET__ only")


if __name__ == "__main__":
    unittest.main()
