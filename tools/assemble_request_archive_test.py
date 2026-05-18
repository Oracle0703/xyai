#!/usr/bin/env python3
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("assemble_request_archive.py")


class AssembleRequestArchiveTest(unittest.TestCase):
    def test_merges_request_and_response_by_archive_id(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "2026-05-15.jsonl"
            output = root / "assembled.jsonl"
            source.write_text(
                "\n".join(
                    [
                        json.dumps(
                            {
                                "archive_id": "a1",
                                "event": "request",
                                "timestamp": "2026-05-15T10:00:00Z",
                                "path": "/v1/responses",
                                "model": "gpt-5",
                                "body": '{"input":"hello"}',
                            },
                            ensure_ascii=False,
                        ),
                        json.dumps(
                            {
                                "archive_id": "a1",
                                "event": "response",
                                "timestamp": "2026-05-15T10:00:01Z",
                                "status": 200,
                                "body": '{"output":"world"}',
                            },
                            ensure_ascii=False,
                        ),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--input",
                    str(source),
                    "--output",
                    str(output),
                ],
                text=True,
                capture_output=True,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            records = [
                json.loads(line)
                for line in output.read_text(encoding="utf-8").splitlines()
                if line.strip()
            ]
            self.assertEqual(len(records), 1)
            self.assertEqual(records[0]["archive_id"], "a1")
            self.assertEqual(records[0]["status"], "complete")
            self.assertEqual(records[0]["request"]["body"], '{"input":"hello"}')
            self.assertEqual(records[0]["response"]["body"], '{"output":"world"}')

    def test_marks_missing_response_as_request_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "2026-05-15.jsonl"
            output = root / "assembled.jsonl"
            source.write_text(
                json.dumps(
                    {
                        "archive_id": "a2",
                        "event": "request",
                        "timestamp": "2026-05-15T10:00:00Z",
                        "body": "{}",
                    }
                )
                + "\n",
                encoding="utf-8",
            )

            result = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPT),
                    "--input",
                    str(source),
                    "--output",
                    str(output),
                ],
                text=True,
                capture_output=True,
            )

            self.assertEqual(result.returncode, 0, result.stderr)
            record = json.loads(output.read_text(encoding="utf-8").strip())
            self.assertEqual(record["status"], "request_only")
            self.assertIsNone(record["response"])


if __name__ == "__main__":
    unittest.main()

