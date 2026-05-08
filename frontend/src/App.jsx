import { useState, useEffect } from "react";

function App() {
  const [logs, setLogs] = useState([]);
  const [stats, setStats] = useState({ allowed: 0, limited: 0, blocked: 0 });

  useEffect(() => {
    // 1. Connect to the Go API Gateway
    const ws = new WebSocket("ws://localhost:8080/ws");

    ws.onopen = () => console.log("🟢 Connected to Security Gateway");

    // 2. Listen for incoming JSON alerts
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);

      // Add the new log to the top of the list
      setLogs((prevLogs) => [data, ...prevLogs].slice(0, 50)); // Keep last 50

      // Update the counters
      setStats((prev) => ({
        ...prev,
        allowed: data.status === "ALLOWED" ? prev.allowed + 1 : prev.allowed,
        limited:
          data.status === "RATE_LIMITED" ? prev.limited + 1 : prev.limited,
        blocked: data.status === "BLOCKED" ? prev.blocked + 1 : prev.blocked,
      }));
    };

    return () => ws.close(); // Cleanup when component unmounts
  }, []);

  return (
    <div className="min-h-screen p-8 font-mono">
      <h1 className="text-3xl font-bold mb-8 text-cyan-400">
        🛡️ SOC Live Dashboard
      </h1>

      {/* Metrics Row */}
      <div className="grid grid-cols-3 gap-4 mb-8 text-center">
        <div className="bg-slate-800 p-4 rounded border-t-4 border-green-500">
          <h2 className="text-gray-400">Traffic Allowed</h2>
          <p className="text-3xl font-bold text-green-400">{stats.allowed}</p>
        </div>
        <div className="bg-slate-800 p-4 rounded border-t-4 border-yellow-500">
          <h2 className="text-gray-400">Rate Limited (Redis)</h2>
          <p className="text-3xl font-bold text-yellow-400">{stats.limited}</p>
        </div>
        <div className="bg-slate-800 p-4 rounded border-t-4 border-red-500">
          <h2 className="text-gray-400">Threats Blocked (C++)</h2>
          <p className="text-3xl font-bold text-red-500">{stats.blocked}</p>
        </div>
      </div>

      {/* Live Terminal Feed */}
      <div className="bg-black p-4 rounded border border-gray-700 h-[500px] overflow-y-auto shadow-2xl shadow-cyan-900/20">
        <h2 className="text-gray-500 mb-4 border-b border-gray-800 pb-2">
          Live Traffic Feed...
        </h2>
        {logs.map((log, i) => (
          <div key={i} className="mb-2 text-sm flex items-center">
            <span className="text-gray-500 mr-4">
              {new Date().toLocaleTimeString()}
            </span>
            <span
              className={`font-bold w-32 ${
                log.status === "ALLOWED"
                  ? "text-green-500"
                  : log.status === "RATE_LIMITED"
                    ? "text-yellow-500"
                    : "text-red-500"
              }`}
            >
              [{log.status}]
            </span>
            <span className="text-gray-300">IP: {log.ip}</span>
            {log.count > 0 && (
              <span className="ml-4 text-gray-500">
                (Requests: {log.count})
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

export default App;
