export type ConnectionStatus = "idle" | "connecting" | "connected" | "reconnecting" | "failed" | "closed";

export interface ProtocolMetadata {
  minSupportedVersion: number;
  currentVersion: number;
  defaultSubprotocol: string;
  supportedSubprotocols: readonly string[];
}

export interface GeneratedContractMetadata<Protocol extends ProtocolMetadata = ProtocolMetadata> {
  contractFormat: string;
  contractVersion: number;
  moduleName: string;
  moduleVersion: string;
  protocol: Protocol;
}

export interface ConnectionState {
  status: ConnectionStatus;
  error?: Error;
}

export interface ConnectionStateChange<Protocol extends ProtocolMetadata = ProtocolMetadata> {
  previous: ConnectionState;
  current: ConnectionState;
  protocol: Protocol;
}

export type ConnectionStateListener<Protocol extends ProtocolMetadata = ProtocolMetadata> = (
  change: ConnectionStateChange<Protocol>,
) => void;

export type TokenSource = string | (() => string | Promise<string>);

export interface ReconnectOptions {
  enabled?: boolean;
  resubscribe?: boolean;
}

export type ReducerCaller<Name extends string = string, Args = Uint8Array, Result = Uint8Array> = (
  name: Name,
  args: Args,
) => Promise<Result>;

export interface ReducerCallResult<Name extends string = string, Result = Uint8Array> {
  reducerName: Name;
  requestID?: number;
  result?: Result;
}

export interface ReducerCallResultRequestOptions<Result = Uint8Array> {
  decodeResult?: (result: Uint8Array) => Result;
}

export interface EncodedReducerCallOptions<Args = unknown> {
  args?: Args;
}

export interface EncodedReducerCallResultOptions<Args = unknown, Result = Uint8Array>
  extends ReducerCallResultRequestOptions<Result> {
  args?: Args;
}

export type QueryRunner<Result = Uint8Array> = (sql: string) => Promise<Result>;

export type DeclaredQueryRunner<Name extends string = string, Result = Uint8Array> = (
  name: Name,
) => Promise<Result>;

export interface RawDeclaredQueryTable {
  tableName: string;
  rows: Uint8Array;
}

export interface RawDeclaredQueryResult<Name extends string = string> {
  queryName: Name;
  tables: RawDeclaredQueryTable[];
}

export interface DecodedDeclaredQueryTable<TableName extends string = string, Row = unknown> {
  tableName: TableName;
  rows: Row[];
}

export interface DecodedDeclaredQueryResult<Name extends string = string, RowsByName extends object = Record<string, unknown>> {
  queryName: Name;
  tables: Array<{
    [TableName in Extract<keyof RowsByName, string>]: DecodedDeclaredQueryTable<TableName, RowsByName[TableName]>;
  }[Extract<keyof RowsByName, string>]>;
}

export interface DeclaredQueryDecodeOptions<RowsByName extends object = Record<string, unknown>> {
  tableDecoders?: TableRowDecoders<RowsByName>;
  decodeRow?: TableRowDecoder<RowsByName[keyof RowsByName]>;
}

export type SubscriptionUnsubscribe = () => void | Promise<void>;

export interface SubscriptionUpdate<Row = unknown> {
  queryID: number;
  tableName: string;
  inserts: Row[];
  deletes: Row[];
}

export interface SubscriptionHandle<Row = unknown> {
  unsubscribe: SubscriptionUnsubscribe;
  rows: Row[];
}

export interface SubscriptionHandleReturnOptions {
  returnHandle?: boolean;
}

export type ViewSubscriber = <Row = unknown>(
  sql: string,
  options?: DeclaredViewSubscriptionOptions<Row>,
) => Promise<SubscriptionUnsubscribe>;

export type DeclaredViewSubscriber<Name extends string = string> = <Row = unknown>(
  name: Name,
  options?: DeclaredViewSubscriptionOptions<Row>,
) => Promise<SubscriptionUnsubscribe>;

export type DeclaredViewHandleSubscriber<Name extends string = string> = <Row = unknown>(
  name: Name,
  options?: DeclaredViewSubscriptionOptions<Row> & SubscriptionHandleReturnOptions,
) => Promise<SubscriptionHandle<Row>>;

export interface DeclaredViewSubscriptionOptions<Row = unknown> {
  decodeRow?: TableRowDecoder<Row>;
  onInitialRows?: (rows: Row[], meta: { queryID: number; tableName: string }) => void;
  onUpdate?: (update: SubscriptionUpdate<Row>) => void;
}

export type TableSubscriber<TableName extends string = string, RowsByName extends object = Record<string, unknown>, Row = unknown> = (
  tableName: TableName,
  onRows?: (rows: Row[]) => void,
  options?: TableSubscriptionOptions<Row>,
) => Promise<SubscriptionUnsubscribe>;

export interface TableSubscriptionOptions<Row = unknown> {
  decodeRow?: TableRowDecoder<Row>;
  onInitialRows?: (rows: Row[], meta: { queryID: number; tableName: string }) => void;
  onUpdate?: (update: SubscriptionUpdate<Row>) => void;
}

export interface BsatnColumn {
  name: string;
  kind: string;
  nullable?: boolean;
}

export type TableRowDecoder<Row = unknown> = (row: Uint8Array) => Row;

export type TableRowDecoders<RowsByName extends object = Record<string, unknown>> = {
  [TableName in Extract<keyof RowsByName, string>]: TableRowDecoder<RowsByName[TableName]>;
};

export interface ShunterClient<Protocol extends ProtocolMetadata = ProtocolMetadata> {
  readonly state: ConnectionState;
  connect(): Promise<void>;
  dispose(): Promise<void>;
  callReducer: ReducerCaller;
  runQuery: QueryRunner;
  runDeclaredQuery: DeclaredQueryRunner;
  subscribeTable: TableSubscriber;
  subscribeView: ViewSubscriber;
  subscribeDeclaredView: DeclaredViewSubscriber & DeclaredViewHandleSubscriber;
}

export interface CreateShunterClientOptions<Protocol extends ProtocolMetadata = ProtocolMetadata> {
  url: string;
  protocol: Protocol;
  contract?: GeneratedContractMetadata<Protocol>;
  token?: TokenSource;
  reconnect?: ReconnectOptions | false;
  onStateChange?: ConnectionStateListener<Protocol>;
}

export function createShunterClient<Protocol extends ProtocolMetadata = ProtocolMetadata>(
  options: CreateShunterClientOptions<Protocol>,
): ShunterClient<Protocol>;

export function assertGeneratedContractCompatible<Protocol extends ProtocolMetadata = ProtocolMetadata>(
  contract: GeneratedContractMetadata<Protocol>,
  expected?: { moduleName?: string; moduleVersion?: string },
): GeneratedContractMetadata<Protocol>;

export function callReducerWithResult<Name extends string = string>(
  callReducer: ReducerCaller<Name, Uint8Array, Uint8Array>,
  name: Name,
  args: Uint8Array,
  options?: ReducerCallResultRequestOptions,
): Promise<ReducerCallResult<Name>>;

export function decodeBsatnProduct<Row>(
  row: Uint8Array,
  columns: readonly BsatnColumn[],
  map: (values: unknown[]) => Row,
): Row;

export function decodeDeclaredQueryResult<Name extends string = string, RowsByName extends object = Record<string, unknown>>(
  name: Name,
  data: unknown,
  options?: DeclaredQueryDecodeOptions<RowsByName>,
): DecodedDeclaredQueryResult<Name, RowsByName>;
