// Bounded HTTP and authenticated WebSocket ownership; system TLS verification stays enabled.
import Foundation

enum HTTPEvent {
    case head(HTTPURLResponse)
    case bytes(Data)
    case end
    case failed(Error)
}

enum SocketEvent {
    case open(String)
    case text(String)
    case binary(Data)
    case close(Int, String, Bool)
}

final class RemoteNetwork: NSObject, URLSessionDataDelegate, URLSessionWebSocketDelegate {
    private struct Download {
        let id: String
        let task: URLSessionDataTask
        let limit: Int
        let event: (HTTPEvent) -> Void
        var bytes = 0
    }
    private struct Socket {
        let id: String
        let task: URLSessionWebSocketTask
        let event: (SocketEvent) -> Void
    }
    private let lock = NSLock()
    private var downloads: [Int: Download] = [:]
    private var sockets: [Int: Socket] = [:]
    private let queue: OperationQueue = {
        let queue = OperationQueue()
        queue.maxConcurrentOperationCount = 1
        return queue
    }()
    private lazy var session: URLSession = {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.httpCookieStorage = nil
        configuration.httpShouldSetCookies = false
        configuration.urlCache = nil
        configuration.timeoutIntervalForRequest = 30
        configuration.timeoutIntervalForResource = 120
        return URLSession(configuration: configuration, delegate: self, delegateQueue: queue)
    }()

    func start(_ request: URLRequest, id: String, limit: Int = 32 * 1024 * 1024, event: @escaping (HTTPEvent) -> Void) throws {
        let task = session.dataTask(with: request)
        lock.lock()
        if downloads.values.contains(where: { $0.id == id }) { lock.unlock(); task.cancel(); throw RemoteFailure.protocolData }
        downloads[task.taskIdentifier] = Download(id: id, task: task, limit: limit, event: event)
        lock.unlock()
        task.resume()
    }

    func read(_ request: URLRequest, limit: Int) async throws -> (Data, HTTPURLResponse) {
        try await withCheckedThrowingContinuation { continuation in
            var body = Data()
            var response: HTTPURLResponse?
            do {
                try start(request, id: UUID().uuidString, limit: limit) { event in
                    switch event {
                    case .head(let value): response = value
                    case .bytes(let bytes): body.append(bytes)
                    case .end:
                        if let response = response { continuation.resume(returning: (body, response)) }
                        else { continuation.resume(throwing: RemoteFailure.protocolData) }
                    case .failed(let error): continuation.resume(throwing: error)
                    }
                }
            } catch { continuation.resume(throwing: error) }
        }
    }

    func cancel(_ id: String) {
        lock.lock()
        let entry = downloads.values.first(where: { $0.id == id })
        if let entry = entry { downloads.removeValue(forKey: entry.task.taskIdentifier) }
        lock.unlock()
        entry?.task.cancel()
        if let entry = entry { queue.addOperation { entry.event(.failed(URLError(.cancelled))) } }
    }

    func open(_ request: URLRequest, id: String, event: @escaping (SocketEvent) -> Void) throws {
        let task = session.webSocketTask(with: request)
        task.maximumMessageSize = 32 * 1024 * 1024
        lock.lock()
        if sockets.values.contains(where: { $0.id == id }) { lock.unlock(); task.cancel(); throw RemoteFailure.protocolData }
        sockets[task.taskIdentifier] = Socket(id: id, task: task, event: event)
        lock.unlock()
        task.resume()
        Task {
            do {
                while self.socket(task.taskIdentifier) != nil {
                    let message = try await task.receive()
                    switch message {
                    case .string(let text): self.socket(task.taskIdentifier)?.event(.text(text))
                    case .data(let bytes): self.socket(task.taskIdentifier)?.event(.binary(bytes))
                    @unknown default: throw RemoteFailure.protocolData
                    }
                }
            } catch { self.finishSocket(task.taskIdentifier, code: 1006, reason: "", clean: false) }
        }
    }

    func send(_ id: String, text: String?, bytes: Data?) async throws {
        guard let entry = findSocket(id) else { throw RemoteFailure.disconnected }
        if let text = text { try await entry.task.send(.string(text)) }
        else if let bytes = bytes { try await entry.task.send(.data(bytes)) }
        else { throw RemoteFailure.protocolData }
    }

    func close(_ id: String, code: Int, reason: String) {
        guard let entry = findSocket(id) else { return }
        entry.task.cancel(with: URLSessionWebSocketTask.CloseCode(rawValue: code) ?? .normalClosure, reason: Data(reason.utf8))
        finishSocket(entry.task.taskIdentifier, code: code, reason: reason, clean: true)
    }

    func checkSockets() {
        lock.lock()
        let current = Array(sockets.values)
        lock.unlock()
        for entry in current {
            entry.task.sendPing { error in
                if error != nil { self.finishSocket(entry.task.taskIdentifier, code: 1006, reason: "", clean: false) }
            }
        }
    }

    func shutdown() {
        reset()
        session.invalidateAndCancel()
    }

    func reset() {
        lock.lock()
        let http = Array(downloads.values)
        let ws = Array(sockets.values)
        downloads.removeAll()
        sockets.removeAll()
        lock.unlock()
        http.forEach { entry in
            entry.task.cancel()
            queue.addOperation { entry.event(.failed(URLError(.cancelled))) }
        }
        ws.forEach { $0.task.cancel(with: .goingAway, reason: nil); $0.event(.close(1001, "", false)) }
    }

    private func findSocket(_ id: String) -> Socket? {
        lock.lock(); defer { lock.unlock() }
        return sockets.values.first(where: { $0.id == id })
    }
    private func socket(_ task: Int) -> Socket? {
        lock.lock(); defer { lock.unlock() }
        return sockets[task]
    }
    private func finishSocket(_ task: Int, code: Int, reason: String, clean: Bool) {
        lock.lock()
        let entry = sockets.removeValue(forKey: task)
        lock.unlock()
        entry?.task.cancel(with: .goingAway, reason: nil)
        entry?.event(.close(code, reason, clean))
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
                    completionHandler: @escaping (URLSession.ResponseDisposition) -> Void) {
        lock.lock()
        let entry = downloads[dataTask.taskIdentifier]
        lock.unlock()
        guard let entry = entry, let response = response as? HTTPURLResponse else { completionHandler(.cancel); return }
        if response.expectedContentLength > entry.limit {
            lock.lock()
            let removed = downloads.removeValue(forKey: dataTask.taskIdentifier)
            lock.unlock()
            removed?.event(.failed(RemoteFailure.limit))
            completionHandler(.cancel)
            return
        }
        entry.event(.head(response))
        completionHandler(.allow)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        lock.lock()
        guard var entry = downloads[dataTask.taskIdentifier] else { lock.unlock(); return }
        entry.bytes += data.count
        downloads[dataTask.taskIdentifier] = entry
        if entry.bytes > entry.limit { downloads.removeValue(forKey: dataTask.taskIdentifier) }
        lock.unlock()
        if entry.bytes > entry.limit { dataTask.cancel(); entry.event(.failed(RemoteFailure.limit)) }
        else { entry.event(.bytes(data)) }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        lock.lock()
        let entry = downloads.removeValue(forKey: task.taskIdentifier)
        lock.unlock()
        guard let entry = entry else { return }
        if let error = error { entry.event(.failed(error)) }
        else { entry.event(.end) }
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocolName: String?) {
        socket(webSocketTask.taskIdentifier)?.event(.open(protocolName ?? ""))
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        finishSocket(webSocketTask.taskIdentifier, code: closeCode.rawValue,
                     reason: reason.flatMap { String(data: $0, encoding: .utf8) } ?? "", clean: true)
    }
}
