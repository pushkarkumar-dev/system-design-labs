# tasks.py — Built-in benchmark tasks.
#
# HellaSwag-lite:  10 4-choice sentence-completion examples.
# TriviaQA-lite:   10 open-ended factual questions.
# MMLU-lite-math:  10 4-choice MMLU-style math questions.
#
# All examples are hand-crafted to exercise the same reasoning patterns as the
# real benchmarks without requiring a network download or large dataset files.
# Real evals use thousands of examples; these 10-example slices show the harness
# mechanics without a slow download.

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class Task:
    """A collection of evaluation examples with a named metric."""
    name: str
    metric: str                        # "exact_match" | "multiple_choice"
    examples: list[dict[str, Any]] = field(default_factory=list)

    def __len__(self) -> int:
        return len(self.examples)


# ---------------------------------------------------------------------------
# HellaSwag-lite: 10 4-choice sentence-completion examples
# ---------------------------------------------------------------------------
# Format: {question, choices: [A, B, C, D], answer: "A"|"B"|"C"|"D"}
# The model must pick which continuation makes most sense.
HELLASWAG_LITE = Task(
    name="HellaSwag-lite",
    metric="multiple_choice",
    examples=[
        {
            "question": "She poured water into the pot and placed it on the stove. She then",
            "choices": [
                "A. turned on the burner and waited for the water to boil.",
                "B. placed the pot in the refrigerator.",
                "C. filled the pot with sand.",
                "D. put the pot on the floor.",
            ],
            "answer": "A",
        },
        {
            "question": "He laced up his running shoes and stepped outside. He then",
            "choices": [
                "A. sat down on the couch.",
                "B. began jogging down the street.",
                "C. took off his shoes.",
                "D. cooked breakfast.",
            ],
            "answer": "B",
        },
        {
            "question": "The teacher wrote the problem on the blackboard. The students",
            "choices": [
                "A. left the classroom.",
                "B. started talking loudly.",
                "C. took out their notebooks and began working.",
                "D. fell asleep.",
            ],
            "answer": "C",
        },
        {
            "question": "The mechanic lifted the hood of the car and inspected the engine. He noticed",
            "choices": [
                "A. the engine was painted red.",
                "B. a loose wire near the battery.",
                "C. a bird's nest inside the engine.",
                "D. the seats were comfortable.",
            ],
            "answer": "B",
        },
        {
            "question": "After mixing the ingredients together, she placed the dough in a pan. She then",
            "choices": [
                "A. put the pan in the oven to bake.",
                "B. poured the dough down the drain.",
                "C. ate the raw dough.",
                "D. put the pan in the freezer.",
            ],
            "answer": "A",
        },
        {
            "question": "The dog saw the ball rolling across the yard. It immediately",
            "choices": [
                "A. fell asleep.",
                "B. chased after the ball.",
                "C. ignored it and walked away.",
                "D. started barking at the fence.",
            ],
            "answer": "B",
        },
        {
            "question": "He opened the book to the first page and began reading. After a few hours, he",
            "choices": [
                "A. had read several chapters.",
                "B. turned on the television.",
                "C. went for a swim.",
                "D. cooked dinner.",
            ],
            "answer": "A",
        },
        {
            "question": "She noticed the plant's leaves were turning yellow. She decided to",
            "choices": [
                "A. water it more frequently.",
                "B. throw the plant away immediately.",
                "C. paint the leaves green.",
                "D. move the plant to a dark room.",
            ],
            "answer": "A",
        },
        {
            "question": "The programmer found a bug in the code. She",
            "choices": [
                "A. deleted the entire project.",
                "B. ignored it and shipped anyway.",
                "C. carefully debugged and fixed the issue.",
                "D. asked a colleague to write the code from scratch.",
            ],
            "answer": "C",
        },
        {
            "question": "The athlete finished the race in first place. She",
            "choices": [
                "A. started crying and left.",
                "B. crossed the finish line and raised her arms in celebration.",
                "C. stopped before the finish line.",
                "D. ran the race again.",
            ],
            "answer": "B",
        },
    ],
)

# ---------------------------------------------------------------------------
# TriviaQA-lite: 10 open-ended factual questions
# ---------------------------------------------------------------------------
# Format: {question, answer}
# Exact match after normalization (lowercase, strip punctuation).
TRIVIAQA_LITE = Task(
    name="TriviaQA-lite",
    metric="exact_match",
    examples=[
        {"question": "What is the capital of France?", "answer": "Paris"},
        {"question": "How many sides does a hexagon have?", "answer": "6"},
        {"question": "What is the chemical symbol for gold?", "answer": "Au"},
        {"question": "Who wrote Romeo and Juliet?", "answer": "Shakespeare"},
        {"question": "What planet is closest to the Sun?", "answer": "Mercury"},
        {"question": "What is the boiling point of water in Celsius?", "answer": "100"},
        {"question": "How many continents are there on Earth?", "answer": "7"},
        {"question": "What is the largest ocean on Earth?", "answer": "Pacific"},
        {"question": "What is the square root of 144?", "answer": "12"},
        {"question": "In what year did World War II end?", "answer": "1945"},
    ],
)

# ---------------------------------------------------------------------------
# MMLU-lite-math: 10 4-choice math questions in MMLU style
# ---------------------------------------------------------------------------
MMLU_LITE_MATH = Task(
    name="MMLU-lite-math",
    metric="multiple_choice",
    examples=[
        {
            "question": "What is 15% of 200?",
            "choices": [
                "A. 20",
                "B. 25",
                "C. 30",
                "D. 35",
            ],
            "answer": "C",
        },
        {
            "question": "If x + 7 = 12, what is x?",
            "choices": [
                "A. 3",
                "B. 4",
                "C. 5",
                "D. 6",
            ],
            "answer": "C",
        },
        {
            "question": "What is the area of a rectangle with length 8 and width 5?",
            "choices": [
                "A. 13",
                "B. 26",
                "C. 40",
                "D. 80",
            ],
            "answer": "C",
        },
        {
            "question": "What is 2^10?",
            "choices": [
                "A. 512",
                "B. 1024",
                "C. 2048",
                "D. 4096",
            ],
            "answer": "B",
        },
        {
            "question": "What is the sum of angles in a triangle?",
            "choices": [
                "A. 90 degrees",
                "B. 180 degrees",
                "C. 270 degrees",
                "D. 360 degrees",
            ],
            "answer": "B",
        },
        {
            "question": "If a train travels 60 mph for 2.5 hours, how far does it go?",
            "choices": [
                "A. 100 miles",
                "B. 120 miles",
                "C. 150 miles",
                "D. 180 miles",
            ],
            "answer": "C",
        },
        {
            "question": "What is log base 10 of 1000?",
            "choices": [
                "A. 2",
                "B. 3",
                "C. 4",
                "D. 10",
            ],
            "answer": "B",
        },
        {
            "question": "What is 7 factorial (7!)?",
            "choices": [
                "A. 2520",
                "B. 5040",
                "C. 720",
                "D. 40320",
            ],
            "answer": "B",
        },
        {
            "question": "What is the derivative of x^3?",
            "choices": [
                "A. x^2",
                "B. 2x^2",
                "C. 3x^2",
                "D. 3x",
            ],
            "answer": "C",
        },
        {
            "question": "How many prime numbers are less than 20?",
            "choices": [
                "A. 6",
                "B. 7",
                "C. 8",
                "D. 9",
            ],
            "answer": "C",
        },
    ],
)

# Exported registry
ALL_TASKS: dict[str, Task] = {
    "hellaswag-lite": HELLASWAG_LITE,
    "triviaqa-lite": TRIVIAQA_LITE,
    "mmlu-lite-math": MMLU_LITE_MATH,
}
