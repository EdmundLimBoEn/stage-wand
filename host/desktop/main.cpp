#include "window.h"
#include <csignal>

static volatile std::sig_atomic_t stopRequested = 0;
static void requestStop(int) { stopRequested = 1; }

int main(int argc, char **argv) {
    QApplication app(argc, argv);
    std::signal(SIGTERM, requestStop);
    std::signal(SIGINT, requestStop);
    QTimer shutdown;
    QObject::connect(&shutdown, &QTimer::timeout, &app, [&app] { if (stopRequested) app.quit(); });
    shutdown.start(100);
    app.setApplicationName("Stage Wand"); app.setDesktopFileName(service);
    app.setQuitOnLastWindowClosed(false);
    auto bus = QDBusConnection::sessionBus();
    if (!bus.isConnected()) { QMessageBox::critical(nullptr, "Stage Wand", "Cannot connect to the desktop session bus."); return 1; }
    if (!bus.registerService(service)) {
        auto reply = QDBusInterface(service, "/StageWand", service, bus).call("showWindow");
        return reply.type() == QDBusMessage::ErrorMessage ? 1 : 0;
    }
    Window window;
    bus.registerObject("/StageWand", &window, QDBusConnection::ExportAllSlots);
    window.show();
    return app.exec();
}
