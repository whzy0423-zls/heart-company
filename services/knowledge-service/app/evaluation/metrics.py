import math
from dataclasses import dataclass


@dataclass(frozen=True)
class EvaluationCaseResult:
    expected_document_id: str
    retrieved_document_ids: list[str]
    latency_ms: float
    polluted: bool
    citation_correct: bool


def _percentile(values: list[float], percentile: float) -> float:
    ordered = sorted(values)
    index = max(0, math.ceil(percentile * len(ordered)) - 1)
    return ordered[index]


def summarize(results: list[EvaluationCaseResult]) -> dict[str, float | int]:
    if not results:
        raise ValueError("evaluation results must not be empty")
    reciprocal_ranks: list[float] = []
    ndcg: list[float] = []
    recall5 = 0
    recall10 = 0
    for result in results:
        try:
            rank = result.retrieved_document_ids.index(result.expected_document_id) + 1
        except ValueError:
            rank = 0
        recall5 += int(0 < rank <= 5)
        recall10 += int(0 < rank <= 10)
        reciprocal_ranks.append(1 / rank if rank else 0)
        ndcg.append(1 / math.log2(rank + 1) if rank else 0)
    count = len(results)
    latencies = [item.latency_ms for item in results]
    return {
        "count": count,
        "recallAt5": recall5 / count,
        "recallAt10": recall10 / count,
        "mrr": sum(reciprocal_ranks) / count,
        "nDCG": sum(ndcg) / count,
        "pollutionRate": sum(item.polluted for item in results) / count,
        "citationAccuracy": sum(item.citation_correct for item in results) / count,
        "p50LatencyMs": _percentile(latencies, 0.50),
        "p95LatencyMs": _percentile(latencies, 0.95),
    }
