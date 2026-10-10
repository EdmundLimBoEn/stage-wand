import Foundation
import Network

@MainActor
final class LocalSocket {
    private let connection: NWConnection

    init(url: URL, interface: NWInterface? = nil) {
        let tcp = NWProtocolTCP.Options()
        tcp.noDelay = true
        let parameters = NWParameters(tls: nil, tcp: tcp)
        parameters.includePeerToPeer = true
        parameters.requiredInterface = interface
        let websocket = NWProtocolWebSocket.Options()
        websocket.autoReplyPing = true
        websocket.maximumMessageSize = 16_384
        parameters.defaultProtocolStack.applicationProtocols.insert(websocket, at: 0)
        connection = NWConnection(to: .url(url), using: parameters)
        connection.start(queue: .main)
    }

    func send(_ data: Data) async throws {
        let connection = connection
        try await withTaskCancellationHandler {
            try Task.checkCancellation()
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                let metadata = NWProtocolWebSocket.Metadata(opcode: .text)
                let context = NWConnection.ContentContext(identifier: "command", metadata: [metadata])
                connection.send(content: data, contentContext: context, isComplete: true,
                                completion: .contentProcessed { error in
                    if let error { continuation.resume(throwing: error) }
                    else { continuation.resume() }
                })
            }
        } onCancel: {
            connection.cancel()
        }
    }

    func receive() async throws -> Data {
        let connection = connection
        return try await withTaskCancellationHandler {
            while true {
                try Task.checkCancellation()
                let data: Data? = try await withCheckedThrowingContinuation { continuation in
                    connection.receiveMessage { data, context, isComplete, error in
                        if let error { continuation.resume(throwing: error); return }
                        let metadata = context?.protocolMetadata(definition: NWProtocolWebSocket.definition) as? NWProtocolWebSocket.Metadata
                        if metadata?.opcode == .close {
                            continuation.resume(throwing: CancellationError())
                        } else if metadata?.opcode == .text || metadata?.opcode == .binary {
                            continuation.resume(returning: data ?? Data())
                        } else if isComplete && data == nil && metadata == nil {
                            continuation.resume(throwing: CancellationError())
                        } else {
                            continuation.resume(returning: nil)
                        }
                    }
                }
                if let data { return data }
            }
        } onCancel: {
            connection.cancel()
        }
    }

    func cancel() { connection.cancel() }
}
