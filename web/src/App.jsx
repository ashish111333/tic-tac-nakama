import { useEffect, useMemo, useRef, useState } from "react";
import * as nakamajs from "@heroiclabs/nakama-js";

const DEFAULT_HOST = "127.0.0.1";
const DEFAULT_PORT = "7350";
const DEFAULT_SERVER_KEY = "supersecrettestkey";
const MOVE_OPCODE = 1;
const STATE_OPCODE = 100;

function makeClient(config) {
  return new nakamajs.Client(
    config.serverKey,
    config.host,
    config.port,
    config.ssl,
    7000
  );
}

async function authenticateDevice(client, deviceId, username) {
  const attempts = [
    () => client.authenticateDevice(deviceId, true, username),
    () => client.authenticateDevice(deviceId, username, true),
    () => client.authenticateDevice(deviceId, true),
    () => client.authenticateDevice(deviceId)
  ];

  let lastErr;
  for (const attempt of attempts) {
    try {
      const session = await attempt();
      if (session) return session;
    } catch (err) {
      lastErr = err;
    }
  }
  throw lastErr || new Error("device authentication failed");
}

function parsePayload(res) {
  if (!res || !res.payload) return {};
  return res.payload;
}

function parseMatchData(data) {
  if (typeof data === "string") {
    return JSON.parse(data);
  }
  if (data instanceof Uint8Array) {
    return JSON.parse(new TextDecoder().decode(data));
  }
  return data;
}

async function errorMessage(err) {
  if (err instanceof Response) {
    try {
      const text = await err.text();
      if (!text) return `HTTP ${err.status}`;
      try {
        const parsed = JSON.parse(text);
        return parsed.message || parsed.error || text;
      } catch {
        return text;
      }
    } catch {
      return `HTTP ${err.status}`;
    }
  }
  return err?.message || String(err);
}

function randomDeviceId() {
  return `dev-${Math.random().toString(36).slice(2, 12)}`;
}

function pickBestMove(board) {
  const empty = board.map((v, i) => (v === 0 ? i : -1)).filter((i) => i >= 0);
  if (empty.length === 0) return -1;
  const center = empty.find((i) => i === 4);
  if (center !== undefined) return center;
  return empty[Math.floor(Math.random() * empty.length)];
}

export default function App() {
  const [host, setHost] = useState(DEFAULT_HOST);
  const [port, setPort] = useState(DEFAULT_PORT);
  const [serverKey, setServerKey] = useState(DEFAULT_SERVER_KEY);
  const [ssl, setSSL] = useState(false);

  const [username, setUsername] = useState("player_one");
  const [deviceId, setDeviceId] = useState(() => randomDeviceId());

  const [clientCtx, setClientCtx] = useState(null);
  const [matchId, setMatchId] = useState("");
  const [mode, setMode] = useState("classic");
  const [rooms, setRooms] = useState([]);
  const [state, setState] = useState(null);
  const [logs, setLogs] = useState([]);

  const [botEnabled, setBotEnabled] = useState(false);
  const botRef = useRef(null);

  const myUserId = clientCtx?.session?.user_id || "";
  const mySymbol = state?.players?.[myUserId] || 0;
  const isMyTurn = Boolean(state && state.turn_user_id === myUserId && state.status === "in_progress");

  const boardLabels = useMemo(() => ["", "X", "O"], []);

  function pushLog(line) {
    setLogs((prev) => [`${new Date().toLocaleTimeString()}  ${line}`, ...prev].slice(0, 120));
  }

  async function connect() {
    try {
      const config = { host, port, serverKey, ssl };
      const client = makeClient(config);
      const session = await authenticateDevice(client, deviceId, username);
      const socket = client.createSocket(ssl, false);
      socket.onmatchdata = (msg) => {
        if (msg.op_code !== STATE_OPCODE) return;
        const payload = parseMatchData(msg.data);
        setState(payload);
      };
      socket.ondisconnect = () => {
        pushLog("socket disconnected");
      };
      await socket.connect(session, true);

      setClientCtx({ config, client, session, socket });
      pushLog(`connected as ${session.username || username}`);
    } catch (err) {
      pushLog(`connect error: ${err.message || String(err)}`);
    }
  }

  async function callRpc(id, payload) {
    if (!clientCtx) return {};
    const res = await clientCtx.client.rpc(clientCtx.session, id, payload || {});
    return parsePayload(res);
  }

  async function createRoom() {
    try {
      const res = await callRpc("create_room", { mode });
      if (!res.match_id) return;
      await joinMatch(res.match_id);
      pushLog(`created room ${res.match_id}`);
    } catch (err) {
      pushLog(`create room failed: ${await errorMessage(err)}`);
    }
  }

  async function quickMatch() {
    try {
      const res = await callRpc("quick_match", { mode });
      if (!res.match_id) return;
      await joinMatch(res.match_id);
      pushLog(`${res.created ? "created" : "joined"} quick match ${res.match_id}`);
    } catch (err) {
      pushLog(`quick match failed: ${await errorMessage(err)}`);
    }
  }

  async function listRooms() {
    try {
      const res = await callRpc("list_rooms", { mode, only_open: true, limit: 20 });
      setRooms(Array.isArray(res.rooms) ? res.rooms : []);
      pushLog(`fetched ${res.rooms?.length || 0} rooms`);
    } catch (err) {
      pushLog(`list rooms failed: ${await errorMessage(err)}`);
    }
  }

  async function joinMatch(id) {
    if (!clientCtx) return;
    await clientCtx.socket.joinMatch(id);
    setMatchId(id);
    pushLog(`joined match ${id}`);
  }

  async function playCell(index) {
    if (!clientCtx || !matchId) return;
    if (!isMyTurn) return;
    try {
      await clientCtx.socket.sendMatchState(matchId, MOVE_OPCODE, JSON.stringify({ cell: index }));
    } catch (err) {
      pushLog(`move failed: ${await errorMessage(err)}`);
    }
  }

  async function ensureBotJoined() {
    if (!clientCtx || !matchId || botRef.current) return;

    const botDevice = randomDeviceId();
    const botName = `bot_${Math.random().toString(36).slice(2, 6)}`;
    const botClient = makeClient(clientCtx.config);
    const botSession = await authenticateDevice(botClient, botDevice, botName);
    const botSocket = botClient.createSocket(clientCtx.config.ssl, false);

    botSocket.onmatchdata = async (msg) => {
      if (msg.op_code !== STATE_OPCODE) return;
      const payload = parseMatchData(msg.data);
      const botUserId = botSession.user_id;
      if (payload.status !== "in_progress") return;
      if (payload.turn_user_id !== botUserId) return;

      const cell = pickBestMove(payload.board || []);
      if (cell < 0) return;

      setTimeout(async () => {
        try {
          await botSocket.sendMatchState(matchId, MOVE_OPCODE, JSON.stringify({ cell }));
        } catch {
          // best effort bot
        }
      }, 500);
    };

    await botSocket.connect(botSession, true);
    await botSocket.joinMatch(matchId);

    botRef.current = { client: botClient, session: botSession, socket: botSocket };
    pushLog(`bot joined as ${botSession.username || botName}`);
  }

  async function disableBot() {
    const bot = botRef.current;
    if (!bot) return;
    botRef.current = null;
    try {
      await bot.socket.close();
    } catch {
      // ignore
    }
    pushLog("bot disconnected");
  }

  useEffect(() => {
    if (!botEnabled) {
      disableBot();
      return;
    }
    ensureBotJoined().catch(async (err) => pushLog(`bot failed: ${await errorMessage(err)}`));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [botEnabled, matchId]);

  useEffect(() => {
    return () => {
      disableBot();
      if (clientCtx?.socket) {
        clientCtx.socket.close();
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="app">
      <h1>Tic-Tac-Toe Nakama Local UI</h1>

      <section className="panel">
        <h2>Connection</h2>
        <div className="grid four">
          <input value={host} onChange={(e) => setHost(e.target.value)} placeholder="Host" />
          <input value={port} onChange={(e) => setPort(e.target.value)} placeholder="Port" />
          <input value={serverKey} onChange={(e) => setServerKey(e.target.value)} placeholder="Server Key" />
          <label className="checkbox">
            <input type="checkbox" checked={ssl} onChange={(e) => setSSL(e.target.checked)} /> SSL
          </label>
        </div>
        <div className="grid three">
          <input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="Username" />
          <input value={deviceId} onChange={(e) => setDeviceId(e.target.value)} placeholder="Device ID" />
          <button onClick={connect} disabled={Boolean(clientCtx)}>Connect</button>
        </div>
      </section>

      <section className="panel">
        <h2>Matchmaking</h2>
        <div className="row">
          <select value={mode} onChange={(e) => setMode(e.target.value)}>
            <option value="classic">Classic</option>
            <option value="timed">Timed</option>
          </select>
          <button onClick={createRoom} disabled={!clientCtx}>Create Room</button>
          <button onClick={quickMatch} disabled={!clientCtx}>Quick Match</button>
          <button onClick={listRooms} disabled={!clientCtx}>List Rooms</button>
        </div>

        <div className="rooms">
          {rooms.length === 0 && <p>No rooms loaded.</p>}
          {rooms.map((r) => (
            <div key={r.match_id} className="room-row">
              <span>{r.match_id}</span>
              <span>{r.mode}</span>
              <span>{r.players}/2</span>
              <button onClick={() => joinMatch(r.match_id)} disabled={!clientCtx}>Join</button>
            </div>
          ))}
        </div>
      </section>

      <section className="panel">
        <h2>Game</h2>
        <div className="row">
          <span>Match: {matchId || "-"}</span>
          <span>Status: {state?.status || "-"}</span>
          <span>You: {myUserId ? `${myUserId.slice(0, 8)} (${boardLabels[mySymbol] || "?"})` : "-"}</span>
          <span>{isMyTurn ? "Your turn" : "Waiting"}</span>
          {state?.move_deadline_unix ? (
            <span>Deadline: {new Date(state.move_deadline_unix * 1000).toLocaleTimeString()}</span>
          ) : null}
        </div>
        {state?.last_error ? <p className="error">Server: {state.last_error}</p> : null}

        <div className="board">
          {(state?.board || Array(9).fill(0)).map((cell, i) => (
            <button
              key={i}
              className="cell"
              onClick={() => playCell(i)}
              disabled={!isMyTurn || cell !== 0}
            >
              {boardLabels[cell]}
            </button>
          ))}
        </div>

        <div className="row">
          <label className="checkbox">
            <input
              type="checkbox"
              checked={botEnabled}
              onChange={(e) => setBotEnabled(e.target.checked)}
              disabled={!matchId}
            />
            Enable Bot Opponent
          </label>
        </div>
      </section>

      <section className="panel">
        <h2>Logs</h2>
        <div className="logs">
          {logs.map((line, idx) => (
            <div key={`${idx}-${line}`}>{line}</div>
          ))}
        </div>
      </section>
    </div>
  );
}
