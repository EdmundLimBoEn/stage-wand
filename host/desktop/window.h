#pragma once
#include <QApplication>
#include <QClipboard>
#include <QCloseEvent>
#include <QDBusConnection>
#include <QDBusInterface>
#include <QDBusMessage>
#include <QDir>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QMenu>
#include <QMessageBox>
#include <QPainter>
#include <QProcess>
#include <QPushButton>
#include <QSystemTrayIcon>
#include <QTimer>
#include <QVBoxLayout>
#include <sys/prctl.h>
#include <unistd.h>
#include <signal.h>

static const char *service = "systems.edmundlim.StageWand";

inline QIcon wandIcon() {
    QPixmap pix(64, 64); pix.fill(Qt::transparent);
    QPainter p(&pix); p.setRenderHint(QPainter::Antialiasing);
    p.setPen(Qt::NoPen); p.setBrush(QColor("#6750a4")); p.drawRoundedRect(2, 2, 60, 60, 16, 16);
    p.setPen(QPen(Qt::white, 6, Qt::SolidLine, Qt::RoundCap));
    p.drawLine(18, 46, 43, 21); p.drawLine(35, 29, 41, 35);
    p.setPen(QPen(Qt::white, 3, Qt::SolidLine, Qt::RoundCap));
    p.drawLine(20, 13, 20, 25); p.drawLine(14, 19, 26, 19);
    p.drawLine(46, 39, 46, 49); p.drawLine(41, 44, 51, 44);
    return QIcon(pix);
}

class Window : public QWidget {
    Q_OBJECT
    Q_CLASSINFO("D-Bus Interface", "systems.edmundlim.StageWand")
    QProcess host;
    QSystemTrayIcon tray{wandIcon()};
    QMenu menu;
    QLabel *code, *peer, *address, *input, *bluetooth, *discovery, *error;
    QPushButton *kick, *copy, *retry;
    QAction *codeAction, *peerAction, *kickAction;
    QByteArray pending;
    QString diagnostics;
    QTimer startupDeadline{this};
    bool discardingLine = false;
    bool acceptingStatus = false;
    bool quitting = false;
    QString executable;
    void stopHost() {
        quitting = true; acceptingStatus = false; startupDeadline.stop(); tray.hide();
        if (host.state() == QProcess::NotRunning) return;
        host.terminate();
        if (!host.waitForFinished(3000)) { host.kill(); host.waitForFinished(1000); }
    }

    QLabel *label(QVBoxLayout *layout, const QString &text = {}) {
        auto *l = new QLabel(text, this);
        l->setTextFormat(Qt::PlainText); l->setWordWrap(true);
        l->setTextInteractionFlags(Qt::TextSelectableByMouse);
        layout->addWidget(l); return l;
    }
    void fail(const QString &message) {
        acceptingStatus = false; startupDeadline.stop(); pending.clear(); discardingLine = false;
        code->setText("—"); peer->setText("Host unavailable");
        input->clear(); bluetooth->clear(); discovery->clear(); address->clear();
        error->setText(message); error->show();
        codeAction->setText("Host unavailable"); peerAction->setText("Disconnected");
        kick->setEnabled(false); copy->setEnabled(false); kickAction->setEnabled(false);
        retry->show(); tray.setToolTip("Stage Wand · host unavailable"); showWindow();
    }
    void start() {
        if (quitting || host.state() != QProcess::NotRunning) return;
        pending.clear(); discardingLine = false; diagnostics.clear(); error->clear(); error->hide(); retry->hide();
        code->setText("…"); peer->setText("Starting host…");
        input->clear(); bluetooth->clear(); discovery->clear(); address->clear();
        codeAction->setText("Starting…"); peerAction->setText("Waiting for host");
        kick->setEnabled(false); copy->setEnabled(false); kickAction->setEnabled(false);
        tray.setToolTip("Stage Wand · starting host");
        const auto path = executable;
        if (!QFileInfo::exists(path)) { fail("Cannot find stagewand-host beside the Stage Wand application. Reinstall the package."); return; }
        // Do not leave an input-injecting host behind if the desktop process crashes.
        const auto parent = getpid();
        host.setChildProcessModifier([parent] {
            if (prctl(PR_SET_PDEATHSIG, SIGTERM) != 0 || getppid() != parent) _exit(1);
        });
        acceptingStatus = true;
        host.start(path, {"--json-status"});
        startupDeadline.start();
    }
    void applyStatus(const QByteArray &line) {
        const auto document = QJsonDocument::fromJson(line);
        if (!document.isObject()) return;
        const auto s = document.object();
        if (s["type"].toString() != "status" || !s["inputReady"].isBool()) return;
        for (const auto *field : {"code", "peer", "address", "input", "bluetooth", "mdns"}) {
            if (!s[field].isString()) return;
        }
        const auto pin = s["code"].toString();
        if (pin.size() != 4) return;
        for (const auto ch : pin) if (ch < QLatin1Char('0') || ch > QLatin1Char('9')) return;
        startupDeadline.stop();
        const auto remote = s["peer"].toString();
        code->setText(pin);
        peer->setText(remote.isEmpty() ? "Waiting for phone" : "Connected · " + remote);
        address->setText(s["address"].toString());
        input->setText((s["inputReady"].toBool() ? "Input · " : "Input unavailable · ") + s["input"].toString());
        const auto ble = s["bluetooth"].toString();
        bluetooth->setText(ble.startsWith("advertising ") ? "Bluetooth · ready" : "Bluetooth · " + ble);
        discovery->setText("Wi-Fi discovery · " + s["mdns"].toString());
        codeAction->setText("Pairing code: " + pin); peerAction->setText(peer->text());
        kick->setEnabled(true); copy->setEnabled(true); kickAction->setEnabled(true);
        tray.setToolTip("Stage Wand · " + peer->text());
    }
    void readStatus() {
        host.setReadChannel(QProcess::StandardOutput);
        while (host.bytesAvailable() > 0) {
            const auto chunk = host.read(4096);
            for (const auto byte : chunk) {
                if (byte == '\n') {
                    if (!discardingLine && acceptingStatus) applyStatus(pending);
                    pending.clear(); discardingLine = false;
                } else if (!discardingLine) {
                    if (pending.size() < 16384) pending += byte;
                    else { pending.clear(); discardingLine = true; }
                }
            }
        }
    }
    void readDiagnostics() {
        host.setReadChannel(QProcess::StandardError);
        while (host.bytesAvailable() > 0) {
            diagnostics = (diagnostics + QString::fromUtf8(host.read(4096))).right(4096);
        }
        host.setReadChannel(QProcess::StandardOutput);
        if (acceptingStatus) {
            error->setText(diagnostics.trimmed()); error->setVisible(!diagnostics.trimmed().isEmpty());
        }
    }
    void disconnectPhone() {
        if (host.state() != QProcess::Running || !kick->isEnabled()) return;
        if (host.write("k\n") != 2) {
            diagnostics = "Cannot send the disconnect command to the host. Restart the host.";
            host.kill(); fail(diagnostics);
        }
    }
public:
    explicit Window(QString hostPath = QCoreApplication::applicationDirPath() + "/stagewand-host") : executable(std::move(hostPath)) {
        setWindowTitle("Stage Wand"); setWindowIcon(wandIcon()); resize(460, 570);
        auto *layout = new QVBoxLayout(this); layout->setContentsMargins(28, 24, 28, 24); layout->setSpacing(12);
        auto *title = label(layout, "Stage Wand"); auto font = title->font(); font.setPointSize(20); font.setBold(true); title->setFont(font);
        label(layout, "PAIRING CODE"); code = label(layout, "…");
        font = code->font(); font.setPointSize(42); font.setBold(true); font.setFamilies({"monospace"}); code->setFont(font);
        code->setWordWrap(false);
        code->setMinimumHeight(code->fontMetrics().height() + 12);
        code->setSizePolicy(QSizePolicy::Preferred, QSizePolicy::Fixed);
        peer = label(layout, "Starting host…"); address = label(layout);
        input = label(layout); bluetooth = label(layout); discovery = label(layout);
        label(layout, "On your phone, draw the square to unlock Stage Wand, choose Bluetooth direct, and enter this code. For Wi-Fi nearby, use the address above if discovery fails.");
        error = label(layout); error->hide();
        auto *row = new QHBoxLayout;
        copy = new QPushButton("Copy code"); kick = new QPushButton("Disconnect & new code");
        copy->setEnabled(false); kick->setEnabled(false);
        row->addWidget(copy); row->addWidget(kick); layout->addLayout(row);
        retry = new QPushButton("Restart host"); retry->hide(); layout->addWidget(retry);
        auto *quit = new QPushButton("Quit Stage Wand"); layout->addWidget(quit);
        connect(copy, &QPushButton::clicked, this, [this] { QApplication::clipboard()->setText(code->text()); });
        connect(kick, &QPushButton::clicked, this, &Window::disconnectPhone);
        connect(retry, &QPushButton::clicked, this, &Window::start);
        connect(quit, &QPushButton::clicked, qApp, &QApplication::quit);
        menu.addAction("Show Stage Wand", this, &Window::showWindow);
        codeAction = menu.addAction("Starting…"); codeAction->setEnabled(false);
        peerAction = menu.addAction("Waiting for phone"); peerAction->setEnabled(false);
        menu.addSeparator(); kickAction = menu.addAction("Disconnect & new code", this, &Window::disconnectPhone); kickAction->setEnabled(false);
        menu.addAction("Quit Stage Wand", qApp, &QApplication::quit);
        tray.setContextMenu(&menu); tray.setToolTip("Stage Wand"); tray.show();
        connect(&tray, &QSystemTrayIcon::activated, this, [this](auto reason) {
            if (reason == QSystemTrayIcon::Trigger || reason == QSystemTrayIcon::DoubleClick) showWindow();
        });
        connect(&host, &QProcess::readyReadStandardOutput, this, &Window::readStatus);
        connect(&host, &QProcess::readyReadStandardError, this, &Window::readDiagnostics);
        connect(&host, &QProcess::errorOccurred, this, [this](auto e) {
            if (!quitting && e == QProcess::FailedToStart) fail(host.errorString());
        });
        connect(&host, &QProcess::finished, this, [this](int status, QProcess::ExitStatus) {
            if (!quitting) {
                readDiagnostics();
                fail(QString("Host stopped (exit %1). %2").arg(status).arg(diagnostics.trimmed()));
            }
        });
        connect(qApp, &QApplication::aboutToQuit, this, &Window::stopHost);
        startupDeadline.setObjectName("startupDeadline");
        startupDeadline.setSingleShot(true); startupDeadline.setInterval(15000);
        connect(&startupDeadline, &QTimer::timeout, this, [this] {
            diagnostics = "Host did not report a valid status. Restart the host, or reinstall the package if the problem continues.";
            host.kill(); fail(diagnostics);
        });
        QTimer::singleShot(0, this, &Window::start);
    }
    ~Window() override { stopHost(); }
public slots:
    void showWindow() { showNormal(); raise(); activateWindow(); }
protected:
    void closeEvent(QCloseEvent *event) override {
        if (QSystemTrayIcon::isSystemTrayAvailable()) { hide(); event->ignore(); }
        else { event->accept(); QApplication::quit(); }
    }
};
