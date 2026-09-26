"""Return the exact GitHub PR facts required by SteerLine's command provider."""

import json
import subprocess
import sys


def gh(*args):
    result = subprocess.run(["gh", *args], check=True, text=True, capture_output=True)
    return result.stdout.strip()


def query(number, repository):
    raw = json.loads(gh("pr", "view", number, "--repo", repository, "--json",
                        "url,state,mergedAt,mergeCommit,baseRefName,headRefOid"))
    merged_at = raw.get("mergedAt")
    merged = merged_at is not None
    merge_commit = raw.get("mergeCommit")
    if isinstance(merge_commit, dict):
        merge_commit = merge_commit.get("oid")
    return {
        "url": raw["url"],
        "repository": repository,
        "base_branch": raw["baseRefName"],
        "head_revision": raw["headRefOid"],
        "state": "merged" if merged else ("closed_unmerged" if raw["state"] == "CLOSED" else "open"),
        "merged": merged,
        "merged_at": merged_at,
        "merge_commit": merge_commit,
    }


def main():
    if len(sys.argv) != 4:
        raise SystemExit("usage: steerline-pr-provider.py query|close NUMBER OWNER/REPO")
    action, number, repository = sys.argv[1:]
    if action == "close":
        current = query(number, repository)
        if current["merged"]:
            raise RuntimeError("cannot close a merged PR")
        if current["state"] == "open":
            gh("pr", "close", number, "--repo", repository)
    elif action != "query":
        raise SystemExit("unsupported PR action")
    print(json.dumps(query(number, repository), separators=(",", ":")))


if __name__ == "__main__":
    main()
