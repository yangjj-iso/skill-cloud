"""Minimal Skill Cloud skill.

Skills receive their inputs as a JSON object on stdin and write their
output as a JSON object on stdout.
"""
import json
import sys


def main() -> None:
    payload = json.loads(sys.stdin.read() or "{}")
    name = payload.get("name", "world")
    json.dump({"message": f"hello, {name}"}, sys.stdout)


if __name__ == "__main__":
    main()
