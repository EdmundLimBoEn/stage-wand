"""Offline request and polling deadline regression tests."""
import contextlib
import http.client
import io
import signal
import time
import unittest
import urllib.error
from unittest.mock import MagicMock, patch

from test_asc_testflight import testflight

profile = testflight.ascprofile


class AbsoluteDeadlineTests(unittest.TestCase):
    def setUp(self):
        self.stack = contextlib.ExitStack()
        self.addCleanup(self.stack.close)
        self.stack.enter_context(patch.object(profile, 'token', return_value='offline-test-token'))

    def test_connection_is_interrupted_without_replaying_write(self):
        def connect(*args, **kwargs):
            time.sleep(1)
            raise AssertionError('connection was not interrupted')

        with patch.object(profile.urllib.request, 'urlopen', side_effect=connect) as opened:
            with self.assertRaises(TimeoutError):
                profile.call('POST', '/test', deadline=time.monotonic() + 0.1)
        self.assertEqual(opened.call_count, 1)

    def test_connection_and_trickling_body_share_one_deadline(self):
        class TricklingBody(io.BytesIO):
            consumed = 0

            def read(self, size=-1):
                chunks = []
                for _ in range(size):
                    time.sleep(0.02)
                    chunks.append(super().read(1))
                    self.consumed += 1
                return b''.join(chunks)

        stream = TricklingBody(b'HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n' + b' ' * 98 + b'{}')
        sock = MagicMock()
        sock.makefile.return_value = stream
        response = http.client.HTTPResponse(sock)

        def connect(*args, **kwargs):
            time.sleep(0.2)
            response.begin()
            return response

        with patch.object(profile.urllib.request, 'urlopen', side_effect=connect) as opened:
            with self.assertRaises(TimeoutError):
                profile.call('GET', '/test', deadline=time.monotonic() + 0.3)
        # A fresh body-only budget would consume ~14 bytes; without a watchdog
        # all 100 bytes arrive before the late-response check notices expiry.
        self.assertLess(stream.consumed, 10)
        self.assertTrue(stream.closed)
        self.assertEqual(opened.call_count, 1)

    def test_default_request_budget_interrupts_body(self):
        real_setitimer = signal.setitimer

        def shortened_timer(which, seconds, interval=0):
            return real_setitimer(which, 0.1 if seconds == 30 else seconds, interval)

        response = MagicMock()
        response.__enter__.return_value.read.side_effect = lambda: time.sleep(1)
        with patch.object(signal, 'setitimer', side_effect=shortened_timer) as timer, \
                patch.object(profile.urllib.request, 'urlopen', return_value=response) as opened:
            with self.assertRaises(TimeoutError):
                profile.call('POST', '/test')
        timer.assert_any_call(signal.ITIMER_REAL, 30)
        self.assertEqual(opened.call_count, 1)
        response.__exit__.assert_called_once()

    def test_alarm_state_is_restored_on_success_and_errors(self):
        original_handler = signal.getsignal(signal.SIGALRM)
        original_timer = signal.getitimer(signal.ITIMER_REAL)
        handler = lambda signum, frame: None
        try:
            signal.signal(signal.SIGALRM, handler)
            for failure in (None, ValueError('invalid JSON'), TimeoutError('expired')):
                with self.subTest(failure=failure):
                    signal.setitimer(signal.ITIMER_REAL, 60, 5)
                    response = MagicMock()
                    response.__enter__.return_value.read.return_value = b'{}'
                    response.__enter__.return_value.read.side_effect = failure
                    with patch.object(profile.urllib.request, 'urlopen', return_value=response):
                        if failure is None:
                            self.assertEqual(profile.call('POST', '/test'), {})
                        else:
                            with self.assertRaises(type(failure)):
                                profile.call('POST', '/test')
                    self.assertIs(signal.getsignal(signal.SIGALRM), handler)
                    remaining, interval = signal.getitimer(signal.ITIMER_REAL)
                    self.assertGreater(remaining, 59)
                    self.assertLessEqual(remaining, 60)
                    self.assertEqual(interval, 5)
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0)
            signal.signal(signal.SIGALRM, original_handler)
            signal.setitimer(signal.ITIMER_REAL, *original_timer)


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
