"""
Tests for the Prompt Optimization Framework.

v0 tests (6):
  - build_prompt includes all input fields
  - call_llm is called exactly once per forward()
  - parse_output extracts output fields
  - ChainOfThought adds reasoning prefix
  - Pipeline chains modules (output feeds next)
  - Signature field order preserved in prompt

v1 tests (4):
  - Bootstrapper selects correct (metric-passing) examples
  - FewShotModule prompt includes demos
  - Compiled module outperforms zero-shot on test set (with biased mock LLM)
  - evaluate() returns correct accuracy

v2 tests (4):
  - Instruction optimization selects best from candidates
  - Random search improves over baseline
  - Budget exceeded raises BudgetExceededError
  - PromptHistory records all trials
"""

from __future__ import annotations

import pytest

from src.v0_signature import ChainOfThought, Module, Pipeline, Signature
from src.v1_bootstrap import (
    Bootstrapper,
    Example,
    FewShotModule,
    exact_match_metric,
)
from src.v2_teleprompter import (
    BootstrapFewShotWithRandomSearch,
    BudgetExceededError,
    OptimizeInstructions,
)
from src.evaluate import evaluate


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def make_mock_llm(response: str = "42"):
    """Returns a mock LLM that always returns `response` and counts calls."""
    calls = {"n": 0}

    def _llm(prompt: str) -> str:
        calls["n"] += 1
        return response

    _llm.calls = calls  # type: ignore[attr-defined]
    return _llm


def make_qa_signature() -> Signature:
    return Signature(
        inputs=["question"],
        outputs=["answer"],
        instructions="Answer the question concisely.",
    )


# ---------------------------------------------------------------------------
# v0 tests
# ---------------------------------------------------------------------------


class TestV0Signature:
    def test_build_prompt_includes_all_input_fields(self):
        sig = Signature(inputs=["question", "context"], outputs=["answer"])
        module = Module(sig)
        prompt = module.build_prompt({"question": "What is 2+2?", "context": "Math"})
        assert "question: What is 2+2?" in prompt
        assert "context: Math" in prompt

    def test_call_llm_called_once_per_forward(self):
        sig = make_qa_signature()
        mock = make_mock_llm("answer: 42")
        module = Module(sig, llm_fn=mock)
        module.forward(question="What is 2+2?")
        assert mock.calls["n"] == 1

    def test_parse_output_extracts_fields(self):
        sig = Signature(inputs=["question"], outputs=["answer", "confidence"])
        module = Module(sig)
        response = "answer: Paris\nconfidence: high"
        result = module.parse_output(response)
        assert result["answer"] == "Paris"
        assert result["confidence"] == "high"

    def test_chain_of_thought_adds_reasoning_prefix(self):
        sig = make_qa_signature()
        cot = ChainOfThought(sig)
        prompt = cot.build_prompt({"question": "What is the capital of France?"})
        assert "reasoning: Let's think step by step." in prompt
        # reasoning must appear before the output field
        reasoning_pos = prompt.index("reasoning:")
        answer_pos = prompt.index("answer:")
        assert reasoning_pos < answer_pos

    def test_pipeline_chains_modules(self):
        """Output of module 1 becomes input of module 2."""
        sig1 = Signature(inputs=["question"], outputs=["intermediate"])
        sig2 = Signature(inputs=["intermediate"], outputs=["final"])

        # Module 1 returns fixed text; module 2 can read it
        module1 = Module(sig1, llm_fn=lambda p: "intermediate: step done")
        module2 = Module(sig2, llm_fn=lambda p: "final: result")

        pipeline = Pipeline()
        pipeline.add(module1).add(module2)

        result = pipeline.forward(question="test")
        assert "intermediate" in result
        assert "final" in result

    def test_signature_field_order_preserved(self):
        sig = Signature(inputs=["a", "b", "c"], outputs=["x", "y"])
        module = Module(sig)
        prompt = module.build_prompt({"a": "1", "b": "2", "c": "3"})
        pos_a = prompt.index("a: 1")
        pos_b = prompt.index("b: 2")
        pos_c = prompt.index("c: 3")
        assert pos_a < pos_b < pos_c


# ---------------------------------------------------------------------------
# v1 tests
# ---------------------------------------------------------------------------


class TestV1Bootstrap:
    def test_bootstrapper_selects_correct_examples(self):
        """Bootstrapper only keeps examples where metric returns True."""
        sig = make_qa_signature()
        # Mock LLM that returns "answer: yes" for "good" inputs, "answer: no" for bad
        def selective_llm(prompt: str) -> str:
            if "good" in prompt:
                return "answer: yes"
            return "answer: no"

        module = Module(sig, llm_fn=selective_llm)

        trainset = [
            Example({"question": "Is this good?"}, {"answer": "yes"}),
            Example({"question": "Is this bad?"}, {"answer": "yes"}),  # llm returns "no" — fail
            Example({"question": "Is this good again?"}, {"answer": "yes"}),
        ]

        bootstrapper = Bootstrapper(n_demos=5)
        demos = bootstrapper.bootstrap(module, trainset, exact_match_metric)

        # Only 2 examples pass (the "good" ones)
        assert len(demos) == 2
        for demo in demos:
            assert demo.label is True

    def test_few_shot_prompt_includes_demos(self):
        sig = make_qa_signature()
        module = FewShotModule(sig, llm_fn=make_mock_llm())
        module.set_demos([
            Example({"question": "What is 1+1?"}, {"answer": "2"}),
            Example({"question": "What is 2+2?"}, {"answer": "4"}),
        ])
        prompt = module.build_prompt({"question": "What is 3+3?"})
        assert "What is 1+1?" in prompt
        assert "What is 2+2?" in prompt
        assert "What is 3+3?" in prompt

    def test_compiled_module_outperforms_zero_shot(self):
        """
        With a biased mock LLM that returns the right answer when shown examples,
        the compiled few-shot module should score higher than zero-shot.
        """
        sig = make_qa_signature()

        # Zero-shot LLM always guesses "answer: wrong"
        zero_shot_llm = lambda prompt: "answer: wrong"

        # Few-shot LLM: if demos are present in prompt, return correct answer
        def few_shot_llm(prompt: str) -> str:
            if "Example 1" in prompt:
                return "answer: correct"
            return "answer: wrong"

        evalset = [
            Example({"question": "Q1"}, {"answer": "correct"}),
            Example({"question": "Q2"}, {"answer": "correct"}),
            Example({"question": "Q3"}, {"answer": "correct"}),
        ]

        zero_shot = Module(sig, llm_fn=zero_shot_llm)
        zero_result = evaluate(zero_shot, evalset, exact_match_metric)

        few_shot = FewShotModule(sig, llm_fn=few_shot_llm)
        few_shot.set_demos([
            Example({"question": "demo Q"}, {"answer": "correct"}),
        ])
        few_result = evaluate(few_shot, evalset, exact_match_metric)

        assert few_result.accuracy > zero_result.accuracy

    def test_evaluate_returns_correct_accuracy(self):
        sig = make_qa_signature()
        # LLM returns "answer: yes" regardless
        module = Module(sig, llm_fn=lambda p: "answer: yes")

        evalset = [
            Example({"question": "Q1"}, {"answer": "yes"}),   # correct
            Example({"question": "Q2"}, {"answer": "yes"}),   # correct
            Example({"question": "Q3"}, {"answer": "no"}),    # wrong
            Example({"question": "Q4"}, {"answer": "yes"}),   # correct
        ]
        result = evaluate(module, evalset, exact_match_metric)

        assert result.total == 4
        assert result.correct == 3
        assert abs(result.accuracy - 0.75) < 1e-9


# ---------------------------------------------------------------------------
# v2 tests
# ---------------------------------------------------------------------------


class TestV2Teleprompter:
    def test_instruction_optimization_selects_best(self):
        """OptimizeInstructions picks the instruction that maximises val accuracy."""
        sig = Signature(inputs=["question"], outputs=["answer"], instructions="bad instruction")

        # LLM that returns "answer: correct" only when prompted with a specific instruction
        def biased_llm(prompt: str) -> str:
            if "Answer correctly" in prompt:
                return "answer: correct"
            return "answer: wrong"

        valset = [
            Example({"question": "Q"}, {"answer": "correct"}),
            Example({"question": "Q2"}, {"answer": "correct"}),
        ]

        optimizer = OptimizeInstructions(
            trainset=[],
            metric=exact_match_metric,
            candidate_instructions=["bad instruction", "Answer correctly", "another bad one"],
            val_subset_size=2,
            max_llm_calls=100,
        )
        best_module, history = optimizer.compile(sig, biased_llm, valset)

        assert len(history) == 3
        best_trial = max(history, key=lambda h: h.val_accuracy)
        assert best_trial.instruction == "Answer correctly"
        assert best_trial.val_accuracy == 1.0

    def test_random_search_improves_over_baseline(self):
        """After random search, best module accuracy >= zero-shot accuracy."""
        sig = make_qa_signature()

        # Mock LLM: returns correct answer when demos are shown
        def llm(prompt: str) -> str:
            if "Example 1" in prompt:
                return "answer: correct"
            return "answer: wrong"

        trainset = [
            Example({"question": "Q"}, {"answer": "correct"}),
            Example({"question": "Q2"}, {"answer": "correct"}),
            Example({"question": "Q3"}, {"answer": "correct"}),
        ]
        valset = [
            Example({"question": "V1"}, {"answer": "correct"}),
            Example({"question": "V2"}, {"answer": "correct"}),
        ]

        base = Module(sig, llm_fn=llm)
        zero_result = evaluate(base, valset, exact_match_metric)

        optimizer = BootstrapFewShotWithRandomSearch(
            max_bootstrapped_demos=2,
            num_trials=3,
            max_llm_calls=200,
        )
        best_module, history = optimizer.compile(base, trainset, valset, exact_match_metric)
        best_result = evaluate(best_module, valset, exact_match_metric)

        assert best_result.accuracy >= zero_result.accuracy
        assert len(history) == 3

    def test_budget_exceeded_raises_error(self):
        """BudgetExceededError is raised when max_llm_calls is exceeded."""
        sig = make_qa_signature()
        module = Module(sig, llm_fn=make_mock_llm())

        trainset = [Example({"question": f"Q{i}"}, {"answer": "x"}) for i in range(20)]
        valset = [Example({"question": f"V{i}"}, {"answer": "x"}) for i in range(10)]

        optimizer = BootstrapFewShotWithRandomSearch(
            max_bootstrapped_demos=5,
            num_trials=10,
            max_llm_calls=2,  # very small budget — will be exceeded quickly
        )

        with pytest.raises(BudgetExceededError):
            optimizer.compile(module, trainset, valset, exact_match_metric)

    def test_prompt_history_records_all_trials(self):
        """PromptHistory contains one entry per trial."""
        sig = make_qa_signature()
        module = Module(sig, llm_fn=make_mock_llm("answer: x"))

        trainset = [Example({"question": "Q"}, {"answer": "x"})]
        valset = [Example({"question": "V"}, {"answer": "x"})]

        optimizer = BootstrapFewShotWithRandomSearch(
            max_bootstrapped_demos=1,
            num_trials=5,
            max_llm_calls=500,
        )
        _, history = optimizer.compile(module, trainset, valset, exact_match_metric)

        assert len(history) == 5
        for i, entry in enumerate(history):
            assert entry.iteration == i
            assert 0.0 <= entry.val_accuracy <= 1.0
