from app.evaluation.metrics import EvaluationCaseResult, summarize
from app.evaluation.dataset import query_excerpt


def test_summary_calculates_recall_mrr_ndcg_and_pollution() -> None:
    summary = summarize([
        EvaluationCaseResult("a", ["a", "x"], latency_ms=100, polluted=False, citation_correct=True),
        EvaluationCaseResult("b", ["x", "b"], latency_ms=300, polluted=True, citation_correct=False),
        EvaluationCaseResult("c", ["x", "y"], latency_ms=200, polluted=False, citation_correct=False),
    ])

    assert summary["count"] == 3
    assert summary["recallAt5"] == 2 / 3
    assert summary["recallAt10"] == 2 / 3
    assert round(summary["mrr"], 4) == 0.5
    assert summary["pollutionRate"] == 1 / 3
    assert summary["citationAccuracy"] == 1 / 3
    assert summary["p50LatencyMs"] == 200
    assert summary["p95LatencyMs"] == 300


def test_query_excerpt_preserves_source_whitespace_for_exact_lexical_match() -> None:
    assert query_excerpt("  第一行\n第二行  ", length=20) == "第一行\n第二行"
