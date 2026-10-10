import Foundation

@MainActor
final class PeerOwnership {
    private var active: (id: UUID, onDisplaced: @MainActor () -> Void)?

    func claim(id: UUID, onDisplaced: @escaping @MainActor () -> Void) {
        let previous = active
        active = (id, onDisplaced)
        if previous?.id != id { previous?.onDisplaced() }
    }

    @discardableResult
    func release(id: UUID) -> Bool {
        guard active?.id == id else { return false }
        active = nil
        return true
    }

    func contains(id: UUID) -> Bool { active?.id == id }
}
