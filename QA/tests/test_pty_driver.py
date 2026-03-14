from __future__ import annotations

import os
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from pty_driver import PTYDriver


class PTYDriverTest(unittest.TestCase):
    def test_can_read_and_send_input(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            script = Path(tmpdir) / "echo.py"
            script.write_text(
                textwrap.dedent(
                    """
                    import sys
                    print("ready", flush=True)
                    line = input()
                    print(f"echo:{line}", flush=True)
                    """
                ).strip()
                + "\n",
                encoding="utf-8",
            )

            driver = PTYDriver()
            driver.start([sys.executable, str(script)], cwd=tmpdir, env=os.environ.copy())
            opening = driver.read_until_quiet()
            self.assertIn("ready", opening)

            driver.send("hello", append_newline=True)
            response = driver.read_until_quiet()
            self.assertIn("echo:hello", response)

            exit_code = driver.stop()
            self.assertEqual(exit_code, 0)


if __name__ == "__main__":
    unittest.main()
