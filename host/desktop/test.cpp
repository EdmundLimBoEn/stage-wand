#include "window.h"
#include <QFile>
#include <QTemporaryDir>
#include <QtTest>
#include <csignal>
#include <cerrno>

class DesktopTest : public QObject {
    Q_OBJECT
    bool hasLabel(Window &w, const QString &text) {
        for (auto *label : w.findChildren<QLabel *>()) if (label->text() == text) return true;
        return false;
    }
private slots:
    void statusKickAndShutdown() {
        QTemporaryDir dir;
        const auto path = dir.path() + "/stagewand-host";
        QFile script(path); QVERIFY(script.open(QIODevice::WriteOnly));
        script.write("#!/bin/sh\necho $$ > \"$0.pid\"\nprintf '%s\\n' '{\"type\":\"status\",\"code\":\"0012\",\"peer\":\"phone\",\"address\":\"192.0.2.1:8787\",\"inputReady\":true,\"input\":\"ready\",\"bluetooth\":\"advertising uuid\"}'\nwhile read -r command; do\nif [ \"$command\" = k ]; then\nprintf '%s\\n' '{\"type\":\"status\",\"code\":\"0034\",\"peer\":\"\",\"inputReady\":false,\"input\":\"permission denied\"}'\nfi\ndone\n");
        script.close(); QVERIFY(script.setPermissions(QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner));
        qint64 pid = 0;
        {
            Window w(path);
            QTRY_VERIFY(hasLabel(w, "0012"));
            w.show();
            for (auto *label : w.findChildren<QLabel *>()) if (label->text() == "0012") QVERIFY(label->height() >= label->fontMetrics().height());
            QVERIFY(hasLabel(w, "Connected · phone"));
            QVERIFY(hasLabel(w, "Bluetooth · ready"));
            QPushButton *kick = nullptr;
            for (auto *button : w.findChildren<QPushButton *>()) if (button->text() == "Disconnect & new code") kick = button;
            QVERIFY(kick); QVERIFY(kick->isEnabled()); kick->click();
            QTRY_VERIFY(hasLabel(w, "0034")); QVERIFY(hasLabel(w, "Waiting for phone"));
            QVERIFY(hasLabel(w, "Input unavailable · permission denied"));
            QFile pidFile(path + ".pid"); QVERIFY(pidFile.open(QIODevice::ReadOnly)); pid = pidFile.readAll().trimmed().toLongLong(); QVERIFY(pid > 0);
        }
        QVERIFY(::kill(pid, 0) == -1 && errno == ESRCH);
    }
    void missingHost() {
        Window w("/nonexistent-stagewand-test-host");
        QTRY_VERIFY(hasLabel(w, "Host unavailable"));
        for (auto *button : w.findChildren<QPushButton *>()) if (button->text() == "Copy code") QVERIFY(!button->isEnabled());
    }
};
QTEST_MAIN(DesktopTest)
#include "test.moc"
