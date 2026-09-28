import re


_TOKENS = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff]+|[a-zA-Z0-9]+")


def search_tokens(text: str) -> str:
    """Stable CJK bigrams and lowercase words for managed lexical indexes."""
    tokens: dict[str, None] = {}
    for match in _TOKENS.finditer(text):
        word = match.group()
        if "\u3400" <= word[0] <= "\u9fff":
            pieces = [word] if len(word) == 1 else (word[index:index + 2] for index in range(len(word) - 1))
        else:
            pieces = [word.lower()]
        for piece in pieces:
            tokens[piece] = None
    return " ".join(tokens)
