#!/usr/bin/env python3
# <What it finds, from which source, and what it writes.> Standard
# library only, so it runs on any server with python3.
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BASE = "https://api.example.com"
UA = "gosint-example/1.0"


def emit(obj: dict) -> None:
    # One JSON object per line, flushed so the log follows the run.
    print(json.dumps(obj, ensure_ascii=False), flush=True)


def log(msg: str, level: str = "info") -> None:
    emit({"log": msg, "level": level})


def get_json(url: str, params: dict | None = None, headers: dict | None = None, tries: int = 3):
    """GET with a few retries on network errors, 429 and 5xx."""
    if params:
        url = f"{url}?{urllib.parse.urlencode(params)}"
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "application/json", **(headers or {})})
    for attempt in range(tries):
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.load(resp)
        except urllib.error.HTTPError as e:
            if e.code != 429 and e.code < 500 or attempt == tries - 1:
                raise
        except (urllib.error.URLError, TimeoutError):
            if attempt == tries - 1:
                raise
        time.sleep(2 * (attempt + 1))
    return None


def main() -> None:
    doc = json.load(sys.stdin)
    node, params = doc["input"], doc.get("params") or {}
    props = node.get("properties") or {}

    # The primary field, else the label.
    query = props.get("domain") or node.get("label") or ""
    if not query:
        log("nothing to search by", "warning")
        return
    limit = int(params.get("limit") or 20)
    api_key = params.get("EXAMPLE_API_KEY") or ""

    data = get_json(f"{BASE}/search", {"q": query, "limit": limit},
                    {"Authorization": f"Bearer {api_key}"} if api_key else None)

    found = 0
    for item in (data or {}).get("results") or []:
        emit({"type": "ip", "properties": {"address": item["ip"]}, "relation": "RESOLVES_TO"})
        found += 1

    emit({"update": {"checked_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}})
    log(f"{query}: {found} found")


if __name__ == "__main__":
    try:
        main()
    except Exception as e:  # noqa: BLE001 - the reason belongs in the log
        print(f"example: {e}", file=sys.stderr)
        sys.exit(1)
