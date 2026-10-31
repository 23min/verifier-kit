"""An adapter program for seeds/spine.yaml speaking the adapter protocol, which MODE bends:

  python3 protocol.py MODE

Each mode changes one reply, from the first request of a kind (new, apply or observe) or from
a later one, so that the toolkit's handling of that reply shows. Replies are written as bytes.
"""
import json
import os
import signal
import sys

MODE = sys.argv[1] if len(sys.argv) > 1 else "ok"
state = "closed"
counts = {"new": 0, "apply": 0, "observe": 0}


def door(event, env):
    """The correct answer, and the state afterwards."""
    global state
    if state == "closed" and event == "open":
        if env["role"] != "owner":
            return "rejected"
        state = "open"
    elif state == "closed" and event == "remove":
        state = "gone"
    return "ok"


def send(data, end=b"\n"):
    if isinstance(data, str):
        data = data.encode("utf-8")
    sys.stdout.buffer.write(data + end)
    sys.stdout.buffer.flush()


BENT = {
    # (op, which request of that op, from 1): the raw reply
    "notjson": ("new", 1, b"hello"),
    "empty": ("new", 1, b""),
    "bom": ("new", 1, '\ufeff{"ok": true}'.encode()),
    "trailing": ("new", 1, b'{"ok": true} x'),
    "ideographic-space": ("new", 1, '{"ok": true}\u3000'.encode()),
    "ok-false": ("new", 1, b'{"ok": false}'),
    "ok-1": ("new", 1, b'{"ok": 1}'),
    "two-keys": ("new", 1, b'{"ok": true, "extra": 1}'),
    "list": ("new", 1, b"[]"),
    "string": ("new", 1, b'"ok"'),
    "result-for-new": ("new", 1, b'{"result": "ok"}'),
    "error-new": ("new", 1, b'{"error": "boom"}'),
    "error-and-more": ("new", 1, b'{"error": "boom", "ok": true}'),
    "error-observe": ("observe", 2, b'{"error": {"a": [1, 2.0, null, true], "b": "it\'s"}}'),
    "error-null": ("observe", 1, b'{"error": null}'),
    "error-apply": ("apply", 1, b'{"error": "ValueError: no"}'),
    "dup-keys": ("observe", 1, b'{"state": "open", "state": "closed"}'),
    "state-number": ("observe", 1, b'{"state": 1}'),
    "result-null": ("apply", 1, b'{"result": null}'),
    "ok-twice": ("new", 1, b'{"ok": true, "ok": true}'),
    "compact": ("new", 1, b'{"ok":true}'),
    "spaced": ("observe", 1, b' { "state" :\r"closed" } '),
    "escaped-key": ("observe", 1, b'{"st\\u0061te": "closed"}'),
    "empty-object": ("new", 1, b'{}'),
    "bad-escape": ("observe", 1, b'{"state": "a\\x"}'),
    "nan": ("apply", 1, b'{"result": NaN}'),
    "infinity": ("apply", 2, b'{"result": -Infinity}'),
    "control-char": ("observe", 1, b'{"state": "a\tb"}'),
    "escapes": ("observe", 2, b'{"state": "\\u00e9\\ud83d\\ude00\\n\\t\\"\\\\\\/"}'),
    "bad-utf8": ("observe", 2, b'{"state": "op\xff\xfeen\xe2\x82\xff\xed\xa0\x80"}'),
    "bad-utf8-outside": ("observe", 1, b'{"state": "closed"}\xff'),
    "crlf": None, "cr-split": None, "cr-at-exit": None, "exit-3": None, "exit-after-new": None, "signal": None,
    "echo-apply": None, "no-reply": None, "mixed-endings": None, "ok": None,
}


def main():
    if MODE not in BENT:
        print(f"protocol.py: no mode {MODE}", file=sys.stderr)
        return 2
    print(f"protocol.py {MODE} started", file=sys.stderr)
    for line in sys.stdin.buffer:
        request = json.loads(line)
        op = request["op"]
        counts[op] += 1
        if MODE == "exit-3" and op == "new":
            return 3
        if MODE == "signal" and op == "apply":
            os.kill(os.getpid(), signal.SIGTERM)
        if MODE == "no-reply" and op == "observe" and counts[op] == 2:
            return 0
        global state
        if op == "new":
            state = "closed"
            reply = {"ok": True}
        elif op == "apply":
            reply = {"result": door(request["event"], request["env"])}
            if MODE == "echo-apply":
                reply = {"error": line.decode().rstrip("\n")}
        else:
            reply = {"state": state}
        bent = BENT[MODE]
        if bent and bent[0] == op and bent[1] == counts[op]:
            send(bent[2])
            continue
        end = b"\n"
        if MODE == "crlf" or (MODE == "mixed-endings" and op == "apply"):
            end = b"\r\n"
        if MODE == "cr-split" and op == "new" and counts[op] == 1:  # two replies in one write, split by \r
            send(json.dumps(reply), end=b"\r" + json.dumps({"state": state}).encode() + b"\n")
            continue
        if MODE == "cr-split" and op == "observe" and counts[op] == 1:
            continue                                        # answered already, after the \r
        if MODE == "cr-at-exit" and op == "new":
            send(json.dumps(reply), end=b"\r")
            return 0
        send(json.dumps(reply), end=end)
        if MODE == "exit-after-new" and op == "new":
            return 0
    return 0


if __name__ == "__main__":
    sys.exit(main())
