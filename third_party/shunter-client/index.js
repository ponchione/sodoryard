const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

const TAG_SUBSCRIBE_SINGLE = 1;
const TAG_UNSUBSCRIBE_SINGLE = 2;
const TAG_CALL_REDUCER = 3;
const TAG_ONE_OFF_QUERY = 4;
const TAG_DECLARED_QUERY = 7;
const TAG_SUBSCRIBE_DECLARED_VIEW = 8;

const TAG_IDENTITY_TOKEN = 1;
const TAG_SUBSCRIBE_SINGLE_APPLIED = 2;
const TAG_SUBSCRIPTION_ERROR = 4;
const TAG_TRANSACTION_UPDATE = 5;
const TAG_ONE_OFF_QUERY_RESPONSE = 6;
const TAG_TRANSACTION_UPDATE_LIGHT = 8;

const BSATN_KIND = {
  bool: 0,
  int8: 1,
  uint8: 2,
  int16: 3,
  uint16: 4,
  int32: 5,
  uint32: 6,
  int64: 7,
  uint64: 8,
  float32: 9,
  float64: 10,
  string: 11,
  bytes: 12,
  timestamp: 17,
  array_string: 18,
  uuid: 19,
  duration: 20,
  json: 21,
};

function concat(parts) {
  const total = parts.reduce((sum, part) => sum + part.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function uint32(value) {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setUint32(0, value >>> 0, true);
  return out;
}

function uint64(value) {
  const out = new Uint8Array(8);
  new DataView(out.buffer).setBigUint64(0, BigInt(value), true);
  return out;
}

function bytes(value) {
  const data = value instanceof Uint8Array ? value : new Uint8Array(value);
  return concat([uint32(data.length), data]);
}

function stringBytes(value) {
  return bytes(textEncoder.encode(value));
}

function frame(tag, bodyParts = []) {
  return concat([Uint8Array.of(tag), ...bodyParts]);
}

function normalizeBytes(value) {
  if (value instanceof Uint8Array) return value;
  if (value instanceof ArrayBuffer) return new Uint8Array(value);
  if (ArrayBuffer.isView(value)) return new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  throw new Error("Expected binary data");
}

function reader(data) {
  return {
    data: normalizeBytes(data),
    offset: 0,
    readUint8() {
      if (this.offset + 1 > this.data.length) throw new Error("Shunter frame is truncated");
      return this.data[this.offset++];
    },
    readUint32() {
      if (this.offset + 4 > this.data.length) throw new Error("Shunter frame is truncated");
      const value = new DataView(this.data.buffer, this.data.byteOffset + this.offset, 4).getUint32(0, true);
      this.offset += 4;
      return value;
    },
    readInt64() {
      if (this.offset + 8 > this.data.length) throw new Error("Shunter frame is truncated");
      const value = new DataView(this.data.buffer, this.data.byteOffset + this.offset, 8).getBigInt64(0, true);
      this.offset += 8;
      return value;
    },
    readUint64() {
      if (this.offset + 8 > this.data.length) throw new Error("Shunter frame is truncated");
      const value = new DataView(this.data.buffer, this.data.byteOffset + this.offset, 8).getBigUint64(0, true);
      this.offset += 8;
      return value;
    },
    readBytes() {
      const length = this.readUint32();
      if (this.offset + length > this.data.length) throw new Error("Shunter frame is truncated");
      const value = this.data.slice(this.offset, this.offset + length);
      this.offset += length;
      return value;
    },
    readString() {
      return textDecoder.decode(this.readBytes());
    },
    done(label) {
      if (this.offset !== this.data.length) {
        throw new Error(`${label} has trailing bytes`);
      }
    },
  };
}

function readOptionalString(r) {
  const present = r.readUint8();
  if (present === 0) return undefined;
  if (present !== 1) throw new Error("Invalid optional string tag");
  return r.readString();
}

function readOptionalUint32(r) {
  const present = r.readUint8();
  if (present === 0) return undefined;
  if (present !== 1) throw new Error("Invalid optional uint32 tag");
  return r.readUint32();
}

function readOptionalTableID(r) {
  return readOptionalUint32(r);
}

function decodeRowList(data) {
  const r = reader(data);
  const count = r.readUint32();
  const rows = [];
  for (let i = 0; i < count; i += 1) {
    rows.push(r.readBytes());
  }
  r.done("RowList");
  return rows;
}

function decodeRows(data, decodeRow) {
  const rows = decodeRowList(data);
  return decodeRow ? rows.map((row) => decodeRow(row)) : rows;
}

function decodeSubscriptionUpdates(r) {
  const count = r.readUint32();
  const updates = [];
  for (let i = 0; i < count; i += 1) {
    updates.push({
      queryID: r.readUint32(),
      tableName: r.readString(),
      inserts: r.readBytes(),
      deletes: r.readBytes(),
    });
  }
  return updates;
}

function decodeServerFrame(data) {
  const raw = normalizeBytes(data);
  if (raw.length < 1) throw new Error("Shunter frame is empty");
  const tag = raw[0];
  const r = reader(raw.slice(1));
  switch (tag) {
    case TAG_IDENTITY_TOKEN: {
      const identity = r.data.slice(r.offset, r.offset + 32);
      r.offset += 32;
      const token = r.readString();
      const connectionID = r.data.slice(r.offset, r.offset + 16);
      r.offset += 16;
      r.done("IdentityToken");
      return { tag, message: { identity, token, connectionID } };
    }
    case TAG_SUBSCRIBE_SINGLE_APPLIED: {
      const requestID = r.readUint32();
      r.readUint64();
      const queryID = r.readUint32();
      const tableName = r.readString();
      const rows = r.readBytes();
      r.done("SubscribeSingleApplied");
      return { tag, message: { requestID, queryID, tableName, rows } };
    }
    case TAG_SUBSCRIPTION_ERROR: {
      r.readUint64();
      const requestID = readOptionalUint32(r);
      const queryID = readOptionalUint32(r);
      const tableID = readOptionalTableID(r);
      const error = r.readString();
      r.done("SubscriptionError");
      return { tag, message: { requestID, queryID, tableID, error } };
    }
    case TAG_ONE_OFF_QUERY_RESPONSE: {
      const messageID = r.readBytes();
      const error = readOptionalString(r);
      const tableCount = r.readUint32();
      const tables = [];
      for (let i = 0; i < tableCount; i += 1) {
        tables.push({ tableName: r.readString(), rows: r.readBytes() });
      }
      r.readInt64();
      r.done("OneOffQueryResponse");
      return { tag, message: { messageID, error, tables } };
    }
    case TAG_TRANSACTION_UPDATE:
      return decodeTransactionUpdate(r, tag);
    case TAG_TRANSACTION_UPDATE_LIGHT: {
      const requestID = r.readUint32();
      const update = decodeSubscriptionUpdates(r);
      r.done("TransactionUpdateLight");
      return { tag, message: { requestID, update } };
    }
    default:
      throw new Error(`Unknown Shunter server frame tag ${tag}`);
  }
}

function decodeTransactionUpdate(r, tag) {
  const statusTag = r.readUint8();
  let update = [];
  let error;
  if (statusTag === 0) {
    update = decodeSubscriptionUpdates(r);
  } else if (statusTag === 1) {
    error = r.readString();
  } else {
    throw new Error(`Unknown Shunter transaction status ${statusTag}`);
  }
  r.readInt64();
  r.offset += 32 + 16;
  const reducerName = r.readString();
  const reducerID = r.readUint32();
  const args = r.readBytes();
  const requestID = r.readUint32();
  r.readInt64();
  r.done("TransactionUpdate");
  return { tag, message: { update, error, reducerCall: { reducerName, reducerID, args, requestID } } };
}

function keyForBytes(value) {
  return Array.from(value, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function makeMessageID(sequence) {
  return textEncoder.encode(`msg:${sequence}`);
}

function parseProtocolResult(data) {
  if (data && typeof data === "object" && Array.isArray(data.tables)) return data;
  const { tag, message } = decodeServerFrame(data);
  if (tag !== TAG_ONE_OFF_QUERY_RESPONSE) {
    throw new Error("Expected Shunter one-off query response");
  }
  if (message.error) throw new Error(message.error);
  return message;
}

function ensureWebSocketAvailable() {
  if (typeof WebSocket === "undefined") {
    throw new Error("WebSocket is not available in this environment");
  }
}

class BrowserShunterClient {
  constructor(options) {
    this.options = options;
    this.ws = undefined;
    this.currentState = { status: "idle" };
    this.sequence = 1;
    this.pendingQueries = new Map();
    this.pendingSubscriptions = new Map();
    this.subscriptions = new Map();
    this.callReducer = (name, args) => this.callReducerRaw(name, args);
    this.runQuery = (sql) => this.runOneOffQuery(sql);
    this.runDeclaredQuery = (name) => this.runDeclaredQueryRaw(name);
    this.subscribeTable = (tableName, onRows, options = {}) => (
      this.subscribeView(`SELECT * FROM ${tableName}`, { ...options, onInitialRows: onRows ?? options.onInitialRows })
    );
    this.subscribeView = (sql, options = {}) => this.subscribeRawSQLView(sql, options);
    this.subscribeDeclaredView = (name, options = {}) => this.subscribeDeclaredViewRaw(name, options);
  }

  get state() {
    return this.currentState;
  }

  setState(status, error) {
    const previous = this.currentState;
    this.currentState = error ? { status, error } : { status };
    this.options.onStateChange?.({ previous, current: this.currentState, protocol: this.options.protocol });
  }

  async connect() {
    if (this.ws?.readyState === WebSocket.OPEN) return;
    ensureWebSocketAvailable();
    this.setState("connecting");
    const token = await resolveToken(this.options.token);
    const url = token ? withToken(this.options.url, token) : this.options.url;
    const protocols = [...(this.options.protocol?.supportedSubprotocols ?? [])];
    this.ws = protocols.length > 0 ? new WebSocket(url, protocols) : new WebSocket(url);
    this.ws.binaryType = "arraybuffer";
    await new Promise((resolve, reject) => {
      const ws = this.ws;
      const cleanup = () => {
        ws.removeEventListener("open", onOpen);
        ws.removeEventListener("error", onError);
      };
      const onOpen = () => {
        cleanup();
        this.setState("connected");
        resolve();
      };
      const onError = () => {
        cleanup();
        const error = new Error("Shunter websocket connection failed");
        this.setState("failed", error);
        reject(error);
      };
      ws.addEventListener("open", onOpen);
      ws.addEventListener("error", onError);
      ws.addEventListener("message", (event) => {
        void this.handleMessage(event.data);
      });
      ws.addEventListener("close", () => {
        if (this.currentState.status !== "closed") this.setState("closed");
      });
    });
  }

  async dispose() {
    this.rejectAll(new Error("Shunter client disposed"));
    for (const subscription of this.subscriptions.values()) {
      this.send(frame(TAG_UNSUBSCRIBE_SINGLE, [uint32(this.nextID()), uint32(subscription.queryID)]), true);
    }
    this.subscriptions.clear();
    if (this.ws && this.ws.readyState <= WebSocket.OPEN) {
      this.ws.close();
    }
    this.setState("closed");
  }

  async callReducerRaw(name, args) {
    const requestID = this.nextID();
    this.send(frame(TAG_CALL_REDUCER, [stringBytes(name), bytes(args), uint32(requestID), Uint8Array.of(0)]));
    return new Uint8Array();
  }

  async runOneOffQuery(sql) {
    const messageID = makeMessageID(this.nextID());
    return this.awaitQuery(messageID, frame(TAG_ONE_OFF_QUERY, [bytes(messageID), stringBytes(sql)]));
  }

  async runDeclaredQueryRaw(name) {
    const messageID = makeMessageID(this.nextID());
    return this.awaitQuery(messageID, frame(TAG_DECLARED_QUERY, [bytes(messageID), stringBytes(name)]), name);
  }

  async subscribeRawSQLView(sql, options) {
    return this.subscribeWithFrame(TAG_SUBSCRIBE_SINGLE, [stringBytes(sql)], options);
  }

  async subscribeDeclaredViewRaw(name, options) {
    if (options.returnHandle) {
      const rows = [];
      const unsubscribe = await this.subscribeWithFrame(TAG_SUBSCRIBE_DECLARED_VIEW, [stringBytes(name)], {
        ...options,
        onInitialRows: (initialRows, meta) => {
          rows.splice(0, rows.length, ...initialRows);
          options.onInitialRows?.(initialRows, meta);
        },
        onUpdate: (update) => {
          rows.push(...update.inserts);
          options.onUpdate?.(update);
        },
      });
      return { unsubscribe, rows };
    }
    return this.subscribeWithFrame(TAG_SUBSCRIBE_DECLARED_VIEW, [stringBytes(name)], options);
  }

  async subscribeWithFrame(tag, trailingParts, options = {}) {
    const requestID = this.nextID();
    const queryID = this.nextID();
    const message = frame(tag, [uint32(requestID), uint32(queryID), ...trailingParts]);
    return new Promise((resolve, reject) => {
      this.pendingSubscriptions.set(requestID, { queryID, options, resolve, reject });
      this.send(message);
    });
  }

  awaitQuery(messageID, message, queryName) {
    const key = keyForBytes(messageID);
    return new Promise((resolve, reject) => {
      this.pendingQueries.set(key, { queryName, resolve, reject });
      this.send(message);
    });
  }

  nextID() {
    const id = this.sequence;
    this.sequence = this.sequence >= 0xfffffff0 ? 1 : this.sequence + 1;
    return id;
  }

  send(message, allowClosed = false) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      if (allowClosed) return;
      throw new Error("Shunter websocket is not connected");
    }
    this.ws.send(message);
  }

  async handleMessage(raw) {
    try {
      const data = raw instanceof Blob ? new Uint8Array(await raw.arrayBuffer()) : normalizeBytes(raw);
      const { tag, message } = decodeServerFrame(data);
      if (tag === TAG_ONE_OFF_QUERY_RESPONSE) {
        this.handleQueryResponse(message);
      } else if (tag === TAG_SUBSCRIBE_SINGLE_APPLIED) {
        this.handleSubscribeApplied(message);
      } else if (tag === TAG_SUBSCRIPTION_ERROR) {
        this.handleSubscriptionError(message);
      } else if (tag === TAG_TRANSACTION_UPDATE_LIGHT || tag === TAG_TRANSACTION_UPDATE) {
        this.handleSubscriptionUpdates(message.update ?? []);
      }
    } catch (error) {
      this.setState("failed", error instanceof Error ? error : new Error(String(error)));
    }
  }

  handleQueryResponse(message) {
    const key = keyForBytes(message.messageID);
    const pending = this.pendingQueries.get(key);
    if (!pending) return;
    this.pendingQueries.delete(key);
    if (message.error) {
      pending.reject(new Error(message.error));
      return;
    }
    pending.resolve({ queryName: pending.queryName, tables: message.tables });
  }

  handleSubscribeApplied(message) {
    const pending = this.pendingSubscriptions.get(message.requestID);
    if (!pending) return;
    this.pendingSubscriptions.delete(message.requestID);
    const rows = decodeRows(message.rows, pending.options.decodeRow);
    const subscription = {
      queryID: message.queryID,
      tableName: message.tableName,
      options: pending.options,
    };
    this.subscriptions.set(message.queryID, subscription);
    pending.options.onInitialRows?.(rows, { queryID: message.queryID, tableName: message.tableName });
    pending.resolve(async () => {
      if (!this.subscriptions.delete(message.queryID)) return;
      this.send(frame(TAG_UNSUBSCRIBE_SINGLE, [uint32(this.nextID()), uint32(message.queryID)]), true);
    });
  }

  handleSubscriptionError(message) {
    const error = new Error(message.error);
    if (message.requestID !== undefined) {
      const pending = this.pendingSubscriptions.get(message.requestID);
      if (pending) {
        this.pendingSubscriptions.delete(message.requestID);
        pending.reject(error);
        return;
      }
    }
    this.setState("failed", error);
  }

  handleSubscriptionUpdates(updates) {
    for (const update of updates) {
      const subscription = this.subscriptions.get(update.queryID);
      if (!subscription) continue;
      const decodeRow = subscription.options.decodeRow;
      subscription.options.onUpdate?.({
        queryID: update.queryID,
        tableName: update.tableName,
        inserts: decodeRows(update.inserts, decodeRow),
        deletes: decodeRows(update.deletes, decodeRow),
      });
    }
  }

  rejectAll(error) {
    for (const pending of this.pendingQueries.values()) pending.reject(error);
    for (const pending of this.pendingSubscriptions.values()) pending.reject(error);
    this.pendingQueries.clear();
    this.pendingSubscriptions.clear();
  }
}

async function resolveToken(token) {
  if (!token) return "";
  return typeof token === "function" ? await token() : token;
}

function withToken(rawURL, token) {
  const url = new URL(rawURL, typeof window !== "undefined" ? window.location.href : "http://localhost");
  url.searchParams.set("token", token);
  return url.toString();
}

export function createShunterClient(options) {
  return new BrowserShunterClient(options);
}

export function assertGeneratedContractCompatible(contract, expected = {}) {
  if (contract?.contractFormat !== "shunter.module_contract") {
    throw new Error("Generated Shunter contract is not a module contract");
  }
  if (contract.contractVersion !== 1) {
    throw new Error(`Unsupported Shunter contract version ${contract.contractVersion}`);
  }
  if (expected.moduleName && contract.moduleName !== expected.moduleName) {
    throw new Error(`Shunter module mismatch: expected ${expected.moduleName}, got ${contract.moduleName}`);
  }
  if (expected.moduleVersion && contract.moduleVersion !== expected.moduleVersion) {
    throw new Error(`Shunter module version mismatch: expected ${expected.moduleVersion}, got ${contract.moduleVersion}`);
  }
  return contract;
}

export async function callReducerWithResult(callReducer, name, args, options = {}) {
  const raw = await callReducer(name, args);
  return {
    reducerName: name,
    result: options.decodeResult ? options.decodeResult(raw) : raw,
  };
}

export function decodeBsatnProduct(row, columns, map) {
  const r = reader(row);
  const values = columns.map((column) => decodeBsatnColumn(r, column));
  r.done("BSATN product");
  return map(values);
}

function decodeBsatnColumn(r, column) {
  const expected = BSATN_KIND[column.kind];
  if (expected === undefined) throw new Error(`Unsupported BSATN column kind ${column.kind}`);
  const tag = r.readUint8();
  if (tag !== expected) {
    throw new Error(`BSATN tag mismatch for ${column.name}: expected ${expected}, got ${tag}`);
  }
  if (column.nullable) {
    const present = r.readUint8();
    if (present === 0) return null;
    if (present !== 1) throw new Error(`Invalid nullable marker for ${column.name}`);
  }
  return decodeBsatnPayload(r, tag);
}

function decodeBsatnPayload(r, tag) {
  switch (tag) {
    case BSATN_KIND.bool: {
      const value = r.readUint8();
      if (value !== 0 && value !== 1) throw new Error("Invalid BSATN bool");
      return value === 1;
    }
    case BSATN_KIND.int8:
      return new Int8Array(Uint8Array.of(r.readUint8()).buffer)[0];
    case BSATN_KIND.uint8:
      return r.readUint8();
    case BSATN_KIND.int16: {
      const value = new DataView(r.data.buffer, r.data.byteOffset + r.offset, 2).getInt16(0, true);
      r.offset += 2;
      return value;
    }
    case BSATN_KIND.uint16: {
      const value = new DataView(r.data.buffer, r.data.byteOffset + r.offset, 2).getUint16(0, true);
      r.offset += 2;
      return value;
    }
    case BSATN_KIND.int32: {
      const value = new DataView(r.data.buffer, r.data.byteOffset + r.offset, 4).getInt32(0, true);
      r.offset += 4;
      return value;
    }
    case BSATN_KIND.uint32:
      return r.readUint32();
    case BSATN_KIND.int64:
    case BSATN_KIND.timestamp:
    case BSATN_KIND.duration:
      return r.readInt64();
    case BSATN_KIND.uint64:
      return r.readUint64();
    case BSATN_KIND.float32: {
      const value = new DataView(r.data.buffer, r.data.byteOffset + r.offset, 4).getFloat32(0, true);
      r.offset += 4;
      return value;
    }
    case BSATN_KIND.float64: {
      const value = new DataView(r.data.buffer, r.data.byteOffset + r.offset, 8).getFloat64(0, true);
      r.offset += 8;
      return value;
    }
    case BSATN_KIND.string:
      return r.readString();
    case BSATN_KIND.bytes:
      return r.readBytes();
    case BSATN_KIND.array_string: {
      const count = r.readUint32();
      const values = [];
      for (let i = 0; i < count; i += 1) values.push(r.readString());
      return values;
    }
    case BSATN_KIND.uuid: {
      const value = r.data.slice(r.offset, r.offset + 16);
      r.offset += 16;
      return Array.from(value, (byte) => byte.toString(16).padStart(2, "0")).join("");
    }
    case BSATN_KIND.json:
      return r.readString();
    default:
      throw new Error(`Unsupported BSATN tag ${tag}`);
  }
}

export function decodeDeclaredQueryResult(name, data, options = {}) {
  const result = parseProtocolResult(data);
  return {
    queryName: name,
    tables: result.tables.map((table) => {
      const tableDecoders = options.tableDecoders ?? {};
      const decodeRow = options.decodeRow ?? tableDecoders[table.tableName];
      return {
        tableName: table.tableName,
        rows: decodeRows(table.rows, decodeRow),
      };
    }),
  };
}

export const __shunterClientInternals = {
  decodeRowList,
  decodeServerFrame,
  frame,
  stringBytes,
  uint32,
  uint64,
};
