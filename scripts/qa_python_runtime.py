from __future__ import annotations

import argparse
import re
from pathlib import Path

try:
    import tomllib
except ModuleNotFoundError:
    tomllib = None

CURATED_PYTHON_VERSIONS = (
    "3.11.11",
    "3.10.16",
    "3.9.21",
    "3.8.20",
)


def parse_version(value: str) -> tuple[int, int, int]:
    parts = [int(part) for part in value.split(".")]
    while len(parts) < 3:
        parts.append(0)
    return tuple(parts[:3])


def satisfies(candidate: tuple[int, int, int], token: str) -> bool:
    match = re.fullmatch(r"\s*(<=|>=|==|!=|<|>)\s*([0-9]+(?:\.[0-9]+){0,2})\s*", token)
    if not match:
        raise ValueError(f"unsupported python constraint token: {token!r}")
    operator, version_text = match.groups()
    required = parse_version(version_text)
    return {
        "<": candidate < required,
        "<=": candidate <= required,
        ">": candidate > required,
        ">=": candidate >= required,
        "==": candidate == required,
        "!=": candidate != required,
    }[operator]


def resolve_python_version(pyproject_path: Path | str) -> str:
    path = Path(pyproject_path)
    constraint = extract_python_constraint(path.read_text(encoding="utf-8"))
    tokens = [token.strip() for token in constraint.split(",") if token.strip()]
    for exact in CURATED_PYTHON_VERSIONS:
        candidate = parse_version(exact)
        if all(satisfies(candidate, token) for token in tokens):
            return exact
    raise ValueError(f"no curated Python image satisfies constraint {constraint!r}")


def extract_python_constraint(pyproject_text: str) -> str:
    if tomllib is not None:
        data = tomllib.loads(pyproject_text)
        return str(data["tool"]["poetry"]["dependencies"]["python"]).strip()

    in_poetry_dependencies = False
    for raw_line in pyproject_text.splitlines():
        line = raw_line.strip()
        if line.startswith("["):
            in_poetry_dependencies = line == "[tool.poetry.dependencies]"
            continue
        if not in_poetry_dependencies:
            continue
        match = re.match(r'python\s*=\s*"([^"]+)"', line)
        if match:
            return match.group(1).strip()
        match = re.match(r"python\s*=\s*'([^']+)'", line)
        if match:
            return match.group(1).strip()

    raise KeyError("tool.poetry.dependencies.python")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Resolve curated QA Python runtime versions.")
    subparsers = parser.add_subparsers(dest="command", required=True)

    resolve_parser = subparsers.add_parser(
        "resolve-python-version",
        help="Resolve the highest curated Python image version compatible with a Poetry pyproject.",
    )
    resolve_parser.add_argument("--pyproject", required=True, type=Path)
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    if args.command == "resolve-python-version":
        print(resolve_python_version(args.pyproject))
        return 0
    parser.error(f"unsupported command: {args.command}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
