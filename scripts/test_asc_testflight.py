"""Offline regression checks: python3 -m unittest discover -s scripts -p 'test_*.py'."""
import contextlib
import importlib.util
import io
import os
from pathlib import Path
import re
import subprocess
import tempfile
import textwrap
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('testflight', ROOT / 'scripts/asc-testflight.py')
testflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(testflight)


class EnsureTests(unittest.TestCase):
    def run_ensure(self, state):
        submission = {'attributes': {'betaReviewState': state}} if state else None
        responses = [
            {'data': [{'id': 'build', 'attributes': {'processingState': 'VALID',
                                                    'usesNonExemptEncryption': False}}]},
            {'data': [{'id': 'group', 'attributes': {'name': 'Testers'}}]},
            {'data': [{'id': 'build'}]},
            {'data': submission},
        ]
        if not state:
            responses.append({'data': {'attributes': {'betaReviewState': 'WAITING_FOR_REVIEW'}}})
        with patch.object(testflight, 'call', side_effect=responses) as api:
            self.api = api
            with contextlib.redirect_stdout(io.StringIO()):
                testflight.ensure('1.0', '42', 'Testers')

    def test_rejected_review_fails_with_actionable_message(self):
        with self.assertRaises(SystemExit) as failure:
            self.run_ensure('REJECTED')
        self.assertNotEqual(failure.exception.code, 0)
        message = str(failure.exception)
        for detail in ('REJECTED', '1.0 (42)', 'App Store Connect', 'resolve'):
            self.assertIn(detail, message)
        self.assertTrue(all(call.args[0] == 'GET' for call in self.api.call_args_list))

    def test_existing_approved_and_pending_reviews_are_idempotent(self):
        for state in ('APPROVED', 'WAITING_FOR_REVIEW', 'IN_REVIEW'):
            with self.subTest(state=state):
                self.run_ensure(state)
                self.assertTrue(all(call.args[0] == 'GET' for call in self.api.call_args_list))

    def test_missing_review_is_submitted(self):
        self.run_ensure(None)
        self.assertEqual(self.api.call_args.args[:2], ('POST', '/v1/betaAppReviewSubmissions'))
        self.assertEqual(self.api.call_args.args[2]['data']['relationships']['build']['data']['id'], 'build')


class WorkflowSummaryTests(unittest.TestCase):
    def summary(self, job, archive, export, upload, external):
        workflow = (ROOT / '.github/workflows/ios-testflight.yml').read_text()
        step = workflow.split('      - name: Summary\n', 1)[1].split('\n      - name:', 1)[0]
        self.assertIn('if: always()', step)
        values = {'job.status': job, 'steps.archive.outcome': archive,
                  'steps.export.outcome': export, 'steps.upload.outcome': upload,
                  'steps.external.outcome': external, 'needs.gate.outputs.upload': 'true'}
        env = {'PATH': os.defpath, 'BUILT_VERSION': '1.0', 'BUILT_BUILD_NUMBER': '42',
               'TESTFLIGHT_EXTERNAL_GROUP': 'Testers'}
        for name, expression in re.findall(r'^          (\w+): \$\{\{ (.*?) \}\}$', step, re.M):
            env[name] = values[expression]
        script = textwrap.dedent(step.split('        run: |\n', 1)[1])
        script = re.sub(r'\$\{\{ (.*?) \}\}', lambda m: values[m[1]], script)
        with tempfile.TemporaryDirectory() as temp:
            summary = Path(temp) / 'summary'
            env['GITHUB_STEP_SUMMARY'] = str(summary)
            subprocess.run(['bash', '-euo', 'pipefail', '-c', script], env=env, check=True)
            return summary.read_text()

    def test_summary_reports_actual_outcomes(self):
        cases = [
            ('failure', 'success', 'success', 'success', 'failure'),  # rejected review
            ('failure', 'success', 'success', 'failure', 'skipped'),
            ('failure', 'failure', 'skipped', 'skipped', 'skipped'),
            ('success', 'success', 'success', 'skipped', 'skipped'),  # dry run
            ('success', 'success', 'success', 'success', 'skipped'),  # internal only
            ('success', 'success', 'success', 'success', 'success'),
            ('cancelled', 'cancelled', 'skipped', 'skipped', 'skipped'),
        ]
        for outcomes in cases:
            with self.subTest(outcomes=outcomes):
                result = self.summary(*outcomes)
                for label, outcome in zip(('Workflow result', 'Archive', 'Export', 'Upload', 'External review'), outcomes):
                    self.assertIn(f'- {label}: {outcome}', result)
                self.assertNotIn('(submitted for Beta App Review)', result)


if __name__ == '__main__':
    unittest.main()
