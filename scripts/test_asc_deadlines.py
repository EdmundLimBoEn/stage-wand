"""Offline request and polling deadline regression tests."""
import contextlib
import io
import unittest
import urllib.error
from unittest.mock import MagicMock, patch

from test_asc_testflight import testflight

profile = testflight.ascprofile


class DeadlineTests(unittest.TestCase):
    def setUp(self):
        self.now = 100.0
        self.sleeps = []
        self.stack = contextlib.ExitStack()
        self.addCleanup(self.stack.close)
        self.stack.enter_context(patch.object(profile.time, 'monotonic', side_effect=lambda: self.now))
        self.stack.enter_context(patch.object(profile.time, 'time', side_effect=AssertionError('wall clock used')))
        self.stack.enter_context(patch.object(profile.time, 'sleep', side_effect=self.sleep))
        self.stack.enter_context(patch.object(profile, 'token', return_value='offline-test-token'))
        self.open = self.stack.enter_context(patch.object(profile.urllib.request, 'urlopen'))
        self.response = MagicMock()
        self.response.__enter__.return_value.read.return_value = b'{"data": []}'
        self.open.return_value = self.response

    def sleep(self, seconds):
        self.assertGreaterEqual(seconds, 0)
        self.sleeps.append(seconds)
        self.now += seconds

    def test_finite_default_and_remaining_deadline_timeout(self):
        profile.call('GET', '/test')
        self.assertEqual(self.open.call_args.kwargs['timeout'], 30)
        profile.call('GET', '/test', deadline=102.5)
        self.assertEqual(self.open.call_args.kwargs['timeout'], 2.5)

    def test_expired_deadline_never_opens_request(self):
        with self.assertRaises(TimeoutError):
            profile.call('GET', '/test', deadline=100)
        self.open.assert_not_called()

    def test_transient_failures_have_three_attempt_limit(self):
        for error in (TimeoutError(), urllib.error.URLError('offline'), ConnectionResetError(),
                      urllib.error.HTTPError('/test', 503, 'offline', {}, None)):
            with self.subTest(error=type(error).__name__):
                self.open.reset_mock()
                self.open.side_effect = error
                with self.assertRaises((TimeoutError, urllib.error.URLError, ConnectionError, SystemExit)):
                    profile.call('GET', '/test')
                self.assertEqual(self.open.call_count, 3)

    def test_retry_recovers_and_recalculates_remaining_budget(self):
        self.open.side_effect = [urllib.error.HTTPError('/test', 429, 'offline', {}, None), self.response]
        self.assertEqual(profile.call('GET', '/test', deadline=102.5), {'data': []})
        self.assertEqual([c.kwargs['timeout'] for c in self.open.call_args_list], [2.5, 1.5])

    def test_retry_sleep_cannot_exceed_deadline(self):
        self.open.side_effect = TimeoutError()
        with self.assertRaises(TimeoutError):
            profile.call('GET', '/test', deadline=100.25)
        self.assertEqual(self.sleeps, [0.25])
        self.assertEqual(self.open.call_count, 1)

    def test_permanent_errors_and_writes_are_not_retried(self):
        for method, code in [('GET', 401), ('POST', 503), ('PATCH', 429)]:
            with self.subTest(method=method, code=code):
                self.open.reset_mock()
                self.open.side_effect = urllib.error.HTTPError('/test', code, 'offline', {}, None)
                with self.assertRaises(SystemExit):
                    profile.call(method, '/test')
                self.assertEqual(self.open.call_count, 1)
        self.assertEqual(self.sleeps, [])

    def test_timed_out_write_is_not_replayed(self):
        self.open.side_effect = TimeoutError()
        with self.assertRaises(TimeoutError):
            profile.call('POST', '/test', {'data': {}})
        self.assertEqual(self.open.call_count, 1)
        self.assertEqual(self.open.call_args.kwargs['timeout'], 30)
        self.assertEqual(self.sleeps, [])

    def test_late_response_is_not_accepted(self):
        def late_read():
            self.now = 103
            return b'{}'
        self.response.__enter__.return_value.read.side_effect = late_read
        with self.assertRaises(TimeoutError):
            profile.call('GET', '/test', deadline=102)
        self.assertEqual(self.open.call_count, 1)

    def test_polling_passes_monotonic_deadline_and_caps_final_sleep(self):
        def poll(method, path, *, deadline):
            self.assertEqual(deadline, 2800)
            self.now = 2799.5
            return {'data': []}
        with patch.object(testflight, 'call', side_effect=poll) as api, contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaisesRegex(SystemExit, 'after 45 minutes'):
                testflight.ensure('1.0', '42', 'Testers')
        self.assertEqual(self.sleeps, [0.5])
        self.assertEqual(api.call_count, 1)

    def test_stalled_poll_exits_without_followup_mutations(self):
        def stalled(request, *, timeout):
            self.now += timeout
            raise TimeoutError()
        self.open.side_effect = stalled
        with contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaisesRegex(SystemExit, 'Timed out waiting for build'):
                testflight.ensure('1.0', '42', 'Testers')
        self.assertEqual(self.open.call_count, 3)
        self.assertTrue(all(c.args[0].method == 'GET' for c in self.open.call_args_list))
