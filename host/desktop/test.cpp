#include "window.h"
#include <QFile>
#include <QTemporaryDir>
#include <QtTest>
#include <csignal>
#include <cerrno>

class DesktopTest : public QObject {
    Q_OBJECT
    static bool hasLabel(Window &w, const QString &text) {
        for (auto *label : w.findChildren<QLabel *>()) if (label->text() == text) return true;
        return false;
    }
    static QPushButton *button(Window &w, const QString &text) {
        for (auto *button : w.findChildren<QPushButton *>()) if (button->text() == text) return button;
        return nullptr;
    }
    static QJsonObject status(QString code = "0012", QString peer = "phone") {
        return {{"type", "status"}, {"code", code}, {"peer", peer},
                {"address", "192.0.2.1:8787"}, {"inputReady", true}, {"input", "ready"},
                {"bluetooth", "advertising uuid"}, {"mdns", "advertised _stagewand._tcp"}};
    }
    static QByteArray json(const QJsonObject &object) {
        return QJsonDocument(object).toJson(QJsonDocument::Compact);
    }
    static QByteArray emitLine(QByteArray line) {
        line.replace("'", "'\\''");
        return "printf '%s\\n' '" + line + "'\n";
    }
    static QByteArray emitFragment(QByteArray fragment) {
        fragment.replace("'", "'\\''");
        return "printf '%s' '" + fragment + "'\n";
    }
    static bool writeHost(const QString &path, const QByteArray &body) {
        QFile script(path);
        if (!script.open(QIODevice::WriteOnly)) return false;
        const auto contents = "#!/bin/sh\necho $$ > \"$0.pid\"\n" + body;
        if (script.write(contents) != contents.size()) return false;
        script.close();
        return script.setPermissions(QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner);
    }
private slots:
    void statusKickAndShutdown() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        auto disconnected = status("0034", "");
        disconnected["inputReady"] = false;
        disconnected["input"] = "permission denied";
        disconnected["bluetooth"] = "unavailable (Bluetooth is powered off)";
        QVERIFY(writeHost(path, emitLine(json(status())) +
            "while read -r command; do\nif [ \"$command\" = k ]; then\n" +
            emitLine(json(disconnected)) + "fi\ndone\n"));
        qint64 pid = 0;
        {
            Window w(path);
            QTRY_VERIFY(hasLabel(w, "0012"));
            w.show();
            for (auto *label : w.findChildren<QLabel *>()) if (label->text() == "0012") QVERIFY(label->height() >= label->fontMetrics().height());
            QVERIFY(hasLabel(w, "Connected · phone"));
            QVERIFY(hasLabel(w, "Bluetooth · ready"));
            QVERIFY(hasLabel(w, "Wi-Fi discovery · advertised _stagewand._tcp"));
            auto *kick = button(w, "Disconnect & new code");
            QVERIFY(kick); QVERIFY(kick->isEnabled()); kick->click();
            QTRY_VERIFY(hasLabel(w, "0034")); QVERIFY(hasLabel(w, "Waiting for phone"));
            QVERIFY(hasLabel(w, "Input unavailable · permission denied"));
            QVERIFY(hasLabel(w, "Bluetooth · unavailable (Bluetooth is powered off)"));
            QFile pidFile(path + ".pid"); QVERIFY(pidFile.open(QIODevice::ReadOnly)); pid = pidFile.readAll().trimmed().toLongLong(); QVERIFY(pid > 0);
        }
        QVERIFY(::kill(pid, 0) == -1 && errno == ESRCH);
    }
    void malformedStatus_data() {
        QTest::addColumn<QByteArray>("payload");
        QTest::newRow("missing-fields") << QByteArray("{\"type\":\"status\"}");
        QTest::newRow("invalid-json") << QByteArray("{\"type\":\"status\"");
        QTest::newRow("array") << QByteArray("[]");
        auto object = status(); object["code"] = 1234;
        QTest::newRow("numeric-code") << json(object);
        object = status("123"); QTest::newRow("short-code") << json(object);
        object = status("12a4"); QTest::newRow("nondigit-code") << json(object);
        object = status(QString::fromUtf8("１２３４")); QTest::newRow("non-ascii-code") << json(object);
        object = status(); object["inputReady"] = "true";
        QTest::newRow("string-ready") << json(object);
        object = status(); object.remove("bluetooth");
        QTest::newRow("missing-bluetooth") << json(object);
        object = status(); object["peer"] = QJsonValue::Null;
        QTest::newRow("null-peer") << json(object);
    }
    void malformedStatus() {
        QFETCH(QByteArray, payload);
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        QVERIFY(writeHost(path, emitLine(payload) + "exec sleep 30\n"));
        Window w(path);
        auto *deadline = w.findChild<QTimer *>("startupDeadline");
        QVERIFY(deadline); deadline->setInterval(100);
        QTRY_VERIFY(hasLabel(w, "Host unavailable"));
        QVERIFY(!button(w, "Copy code")->isEnabled());
        QVERIFY(!button(w, "Disconnect & new code")->isEnabled());
    }
    void oversizedFrameAndFragmentedRecovery() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        auto oversized = status("9876", QString(20000, QLatin1Char('x')));
        const auto valid = json(status());
        QVERIFY(writeHost(path, emitLine(json(oversized)) +
            "echo frame-sent >&2\nwhile [ ! -f \"$0.continue\" ]; do sleep 0.02; done\n" +
            emitFragment(valid.left(valid.size() / 2)) + "sleep 0.02\n" +
            emitLine(valid.mid(valid.size() / 2)) + "exec sleep 30\n"));
        Window w(path);
        QTRY_VERIFY(hasLabel(w, "frame-sent"));
        QVERIFY(!button(w, "Copy code")->isEnabled());
        QVERIFY(!hasLabel(w, "9876"));
        QFile resume(path + ".continue"); QVERIFY(resume.open(QIODevice::WriteOnly)); resume.close();
        QTRY_VERIFY(hasLabel(w, "0012"));
        QVERIFY(button(w, "Copy code")->isEnabled());
    }
    void unterminatedOutputTimesOut() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        QVERIFY(writeHost(path, "printf '%100000s' x\nexec sleep 30\n"));
        Window w(path);
        auto *deadline = w.findChild<QTimer *>("startupDeadline");
        QVERIFY(deadline); deadline->setInterval(100);
        QTRY_VERIFY(hasLabel(w, "Host unavailable"));
        QVERIFY(!button(w, "Copy code")->isEnabled());
    }
    void stderrIsBounded() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        QVERIFY(writeHost(path, "printf '%100000s' x >&2\necho diagnostic-end >&2\n" +
            emitLine(json(status())) + "exec sleep 30\n"));
        Window w(path);
        QTRY_VERIFY(hasLabel(w, "0012"));
        bool found = false;
        for (auto *label : w.findChildren<QLabel *>()) {
            if (label->text().contains("diagnostic-end")) {
                QVERIFY(label->text().size() <= 4096); found = true;
            }
        }
        QVERIFY(found);
    }
    void stoppedHostCanRestart() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        QVERIFY(writeHost(path, "echo initialization-failed >&2\nexit 7\n"));
        Window w(path);
        QTRY_VERIFY(hasLabel(w, "Host unavailable"));
        QVERIFY(!button(w, "Copy code")->isEnabled());
        QVERIFY(hasLabel(w, "Host stopped (exit 7). initialization-failed"));
        QVERIFY(writeHost(path, emitLine(json(status("4321", ""))) + "exec sleep 30\n"));
        button(w, "Restart host")->click();
        QVERIFY(!button(w, "Copy code")->isEnabled());
        QTRY_VERIFY(hasLabel(w, "4321"));
        QVERIFY(button(w, "Copy code")->isEnabled());
        QVERIFY(button(w, "Restart host")->isHidden());
        QVERIFY(!hasLabel(w, "Host stopped (exit 7). initialization-failed"));
    }
    void missingHost() {
        Window w("/nonexistent-stagewand-test-host");
        QTRY_VERIFY(hasLabel(w, "Host unavailable"));
        QVERIFY(!button(w, "Copy code")->isEnabled());
    }
    void unexecutableHost() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        QVERIFY(writeHost(path, "exit 0\n"));
        QVERIFY(QFile::setPermissions(path, QFile::ReadOwner | QFile::WriteOwner));
        Window w(path);
        QTRY_VERIFY(hasLabel(w, "Host unavailable"));
        QVERIFY(!button(w, "Copy code")->isEnabled());
        QVERIFY(!button(w, "Restart host")->isHidden());
    }
};
QTEST_MAIN(DesktopTest)
#include "test.moc"
