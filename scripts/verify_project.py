import argparse
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def run(section):
    checks = {
        "backend": [("backend", ["go", "test", "-race", "./..."]),
                    ("backend", ["go", "build", "./..."]),
                    ("backend", ["go", "vet", "./..."])],
        "frontend": [("web", ["npm", "run", "lint"]),
                     ("web", ["npm", "run", "typecheck"]),
                     ("web", ["npm", "test"]),
                     ("web", ["npm", "run", "build"])],
        "audit": [(".", ["python3", "-m", "unittest", "discover", "-s", "scripts", "-p", "test_*.py"])],
    }
    for directory, command in checks[section]:
        subprocess.run(command, cwd=ROOT / directory, check=True)
    print(f"{section} verification passed", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("section", choices=["backend", "frontend", "audit"])
    run(parser.parse_args().section)
