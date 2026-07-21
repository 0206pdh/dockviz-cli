from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path


def _binary_path() -> Path:
    binary_name = "dockviz.exe" if os.name == "nt" else "dockviz"
    binary = Path(__file__).resolve().with_name(binary_name)
    if not binary.is_file():
        raise FileNotFoundError(f"Bundled dockviz binary not found: {binary}")
    return binary


def main() -> int:
    binary = _binary_path()
    argv = [str(binary), *sys.argv[1:]]

    if os.name != "nt":
        os.execv(str(binary), argv)

    completed = subprocess.run(argv, check=False)
    return completed.returncode


if __name__ == "__main__":
    raise SystemExit(main())
