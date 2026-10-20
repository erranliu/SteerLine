"""Read one original Codex turn through its public protocol; never resume it."""
import argparse
import json
import os
import queue
import subprocess
import threading
import time

p = argparse.ArgumentParser()
p.add_argument("--executable", required=True)
p.add_argument("--workspace", required=True)
p.add_argument("--session", required=True)
p.add_argument("--turn", required=True)
p.add_argument("--output", required=True)
a = p.parse_args()
messages = queue.Queue()
with open(a.output + ".stderr", "w", encoding="utf-8") as diagnostic:
    child = subprocess.Popen(
        [a.executable, "app-server", "--stdio"], cwd=a.workspace,
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=diagnostic,
        text=True, encoding="utf-8", creationflags=subprocess.CREATE_NO_WINDOW,
    )
    def read():
        for line in child.stdout:
            messages.put(json.loads(line))
    threading.Thread(target=read, daemon=True).start()
    def call(number, method, params):
        child.stdin.write(json.dumps({"id": number, "method": method, "params": params}) + "\n")
        child.stdin.flush()
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            reply = messages.get(timeout=max(0.01, deadline - time.monotonic()))
            if reply.get("id") == number:
                if "error" in reply:
                    raise RuntimeError(reply["error"])
                return reply["result"]
        raise TimeoutError(method)
    try:
        call(1, "initialize", {"clientInfo": {"name": "steerline_recovery_audit", "version": "1"},
                                "capabilities": {"experimentalApi": True}})
        child.stdin.write('{"method":"initialized","params":{}}\n')
        child.stdin.flush()
        original = call(2, "thread/read", {"threadId": a.session, "includeTurns": True})["thread"]
        if original["id"] != a.session or os.path.normcase(os.path.realpath(original["cwd"])) != os.path.normcase(os.path.realpath(a.workspace)):
            raise RuntimeError("Original session/workspace identity differs")
        turns = [{"id": t["id"], "status": t["status"], "completed_at": t.get("completedAt"),
                  "final_messages": sum(i.get("type") == "agentMessage" and i.get("phase") == "final_answer"
                                        and i.get("delivery") != "async" and bool(i.get("id")) and bool(i.get("text"))
                                        for i in t.get("items", []))} for t in original["turns"]]
        target = [t for t in turns if t["id"] == a.turn]
        result = {"session_id": a.session, "workspace": a.workspace, "turns": turns,
                  "original_completed": len(target) == 1 and target[0]["status"] == "completed"
                  and target[0]["completed_at"] is not None and target[0]["final_messages"] > 0}
        with open(a.output, "w", encoding="utf-8") as output:
            json.dump(result, output, ensure_ascii=False)
        print(json.dumps(result))
    finally:
        child.stdin.close()
        try:
            child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            child.kill()
            child.wait(timeout=5)
