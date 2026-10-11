#!/usr/bin/env python3
# Lists the Wayback Machine's snapshots of a domain, newest first, as
# website entities under ARCHIVED_AT. Standard library only.
import json
import sys
import urllib.parse
import urllib.request

CDX = "https://web.archive.org/cdx/search/cdx"


def main() -> None:
    doc = json.load(sys.stdin)
    domain = doc["input"]["properties"].get("domain") or doc["input"]["label"]
    limit = int(doc["params"].get("limit") or 20)

    query = urllib.parse.urlencode({
        "url": domain,
        "output": "json",
        "fl": "timestamp,original,statuscode",
        "filter": "statuscode:200",
        "collapse": "digest",
        "limit": -limit,
    })
    req = urllib.request.Request(f"{CDX}?{query}", headers={"User-Agent": "gosint-wayback/1.0"})
    with urllib.request.urlopen(req, timeout=60) as resp:
        rows = json.load(resp)

    seen = 0
    for ts, original, _ in rows[1:]:
        print(json.dumps({
            "type": "website",
            "properties": {"url": f"https://web.archive.org/web/{ts}/{original}", "title": f"{original} at {ts}"},
            "relation": "ARCHIVED_AT",
        }))
        seen += 1

    print(json.dumps({"log": f"{domain}: {seen} snapshots"}))


if __name__ == "__main__":
    main()
