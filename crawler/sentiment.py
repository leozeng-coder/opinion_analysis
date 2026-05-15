import json
import jieba.analyse
from snownlp import SnowNLP


def analyse(title: str, content: str) -> tuple[str, float, str]:
    """Return (sentiment_label, sent_score, keywords_json)."""
    text = (title + "。" + content).strip()
    if not text or text == "。":
        return "neutral", 0.0, "[]"

    raw = SnowNLP(text).sentiments   # 0~1
    sent_score = round(raw * 2 - 1, 4)  # map to -1~1

    if raw > 0.6:
        label = "positive"
    elif raw < 0.4:
        label = "negative"
    else:
        label = "neutral"

    keywords = jieba.analyse.extract_tags(text, topK=10)
    return label, sent_score, json.dumps(keywords, ensure_ascii=False)
