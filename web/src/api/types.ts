// TS mirrors of the Go wire types (json tag names, string uuids).

export interface Health {
  status: 'ok' | 'unavailable'
  database: 'sqlite' | 'postgres'
}

export interface Profile {
  id: string
  email: string
  first_name: string
  last_name: string
  avatar_url: string

  // admin administers accounts. account_type is 1 for a local account
  // with a password here, 2 for one an identity provider signs in.
  admin: boolean
  account_type: number
  totp_enabled: boolean
  passkeys: number
}

// AdminUser is an account as the admin panel lists it.
export interface AdminUser extends Profile {
  is_active: boolean
  created_at: string
}

// An entity type an administrator declared as data.
export interface CustomField {
  name: string
  title: string
  description: string
  kind: string
  required: boolean
  primary: boolean
  long: boolean
  options: string[] | null
}

export interface CustomType {
  id: string
  name: string
  title: string
  description: string
  category: string
  icon: string
  color: string
  fields: CustomField[]
  created_at: string
}

// A maintenance job of the node answering.
export interface Job {
  name: string
  schedule: string
  running: boolean
  last_run_at: string | null
  last_error: string
  next_run_at: string | null
  last_duration_ms: number
}

// Invitation offers an address a place on an investigation.
export interface Invitation {
  id: string
  investigation_id: string
  email: string
  role: Role
  invited_by: string
  created_at: string
  expires_at: string
}

// InvitationLookup is what a link is for and how to take it.
export interface InvitationLookup {
  investigation: string
  email: string
  role: Role
  expires_at: string
  has_account: boolean
  signed_in_as: string
}

// OAuthProvider is an identity provider as the admin panel sees it,
// has_secret in place of the secret.
export interface OAuthProvider {
  id: string
  name: string
  slug: string
  type: string
  client_id: string
  has_secret: boolean
  issuer: string
  auth_url: string
  token_url: string
  userinfo_url: string
  scopes: string[]
  enabled: boolean
  hidden: boolean
  auto_register: boolean
  require_email_verified: boolean
  link_by_email: boolean
  allow_private_targets: boolean
  allowed_domains: string[]
  allowed_emails: string[]
  groups_claim: string
  allowed_groups: string[]
  usable: boolean
  callback_url: string
  created_at: string
  updated_at: string
}

// OAuthProviderInput is a provider to create or patch. client_secret
// absent keeps the stored one.
export interface OAuthProviderInput {
  name: string
  slug?: string
  type?: string
  client_id?: string
  client_secret?: string
  issuer?: string
  auth_url?: string
  token_url?: string
  userinfo_url?: string
  scopes?: string[]
  enabled?: boolean
  hidden?: boolean
  auto_register?: boolean
  require_email_verified?: boolean
  link_by_email?: boolean
  allow_private_targets?: boolean
  allowed_domains?: string[]
  allowed_emails?: string[]
  groups_claim?: string
  allowed_groups?: string[]
}

// OAuthTestResult is what the Test button reports.
export interface OAuthTestResult {
  ok: boolean
  error?: string
  issuer?: string
  auth_url?: string
  token_url?: string
  discovered: boolean
  redirect_url: string
  scopes: string[]
  warnings: string[]
}

// AuthInfo is what the sign-in page may offer, public.
export interface AuthInfo {
  passkeys_enabled: boolean
  mail_enabled: boolean
  registration_open: boolean
  // A fresh install with no account, whose first one claims it.
  setup_required: boolean
  providers: LoginProvider[]
}

// LinkedIdentity is an identity provider linked to the caller's account.
export interface LinkedIdentity {
  id: string
  provider_name: string
  provider_slug: string
  email: string
  created_at: string
  last_login_at: string
}

// LoginProvider is one identity provider to sign in with.
export interface LoginProvider {
  slug: string
  name: string
  start_url: string
}

// Passkey is one enrolled WebAuthn credential.
export interface Passkey {
  id: string
  name: string
  created_at: string
  last_used_at?: string
}

// UserSession is one sign-in of the account.
export interface UserSession {
  id: string
  user_agent: string
  client_ip: string
  created_at: string
  expires_at: string
  current: boolean
}

export type Role = 'owner' | 'admin' | 'editor' | 'viewer'

export interface Investigation {
  id: string
  name: string
  description: string
  status: 'active' | 'archived'
  owner_id: string
  created_at: string
  updated_at: string
  current_user_role: Role
}

export interface Member {
  user_id: string
  role: Role
  user: Profile
}

export interface Sketch {
  id: string
  investigation_id: string
  title: string
  description: string
  status: 'active' | 'archived'
  owner_id: string
  created_at: string
  updated_at: string
}

export interface GraphNode {
  id: string
  sketch_id: string
  type: string
  key: string
  label: string
  properties: Record<string, unknown>
  metadata: Record<string, unknown>
  color: string
  icon: string
  image: string
  flag: string
  shape: string
  size: number | null
  x: number | null
  y: number | null
  created_at: string
  updated_at: string
}

export interface GraphEdge {
  id: string
  sketch_id: string
  source: string
  target: string
  label: string
  properties: Record<string, unknown>
  created_at: string
}

export interface Graph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface TypeField {
  name: string
  title: string
  description: string
  kind: string
  elem: string
  ref: string
  required: boolean
  primary: boolean
  long: boolean
  default: unknown
  options: string[] | null

  // vault_key names one of the account's vault keys, never the secret.
  vault_key: boolean

  // secret is masked until shown and encrypted at rest.
  secret: boolean
}

export interface EntityType {
  name: string
  title: string
  description: string
  category: string
  icon: string
  color: string
  detectable: boolean
  fields: TypeField[]
}

export interface TypeCategory {
  key: string
  title: string
  icon: string
}

export interface Detection {
  value: string
  type: string
  label: string
  properties: Record<string, unknown>
  error: string
}

export interface TransformParam {
  name: string
  type: 'string' | 'url' | 'number' | 'select' | 'bool' | 'vaultSecret'
  description: string
  required: boolean
  default: string
  options: { label: string; value: string }[] | null
}

export interface Transform {
  name: string
  category: string
  description: string
  icon: string
  input: string
  output: string
  params: TransformParam[] | null
  template_id: string
  is_public: boolean
  file: string
}

export type ScanStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

export interface Scan {
  id: string
  sketch_id: string
  owner_id: string
  kind: string
  name: string
  status: ScanStatus
  input: Record<string, unknown>
  details: Record<string, unknown>
  error: string
  attempts: number
  created_at: string
  started_at: string | null
  completed_at: string | null
}

export interface LogEntry {
  id: string
  sketch_id: string
  scan_id: string
  level: string
  message: string
  data: Record<string, unknown>
  created_at: string
}

export interface VaultKey {
  id: string
  owner_id: string
  name: string
  created_at: string
  updated_at: string
}

export interface TransformTemplate {
  id: string
  owner_id: string
  name: string
  description: string
  category: string
  input_type: string
  output_type: string
  source: string
  is_public: boolean
  created_at: string
  updated_at: string
}

export interface FlowStep {
  id: string
  transform: string
  after: string
  params: Record<string, string> | null
}

export interface FlowDefinition {
  input: string
  steps: FlowStep[]
  layout: Record<string, unknown> | null
}

export interface Flow {
  id: string
  owner_id: string
  name: string
  description: string
  definition: FlowDefinition
  created_at: string
  updated_at: string
}

export interface ImportResult {
  nodes_created: number
  nodes_skipped: number
  edges_created: number
  edges_skipped: number
  files_created: number
  files_skipped: number
  errors: string[]
}

// InvestigationImportResult is the investigation an import made and the
// sum of what went in.
export interface InvestigationImportResult {
  investigation: Investigation
  sketches: Sketch[]
  nodes_created: number
  edges_created: number
  files_created: number
  analyses_created: number
  errors: string[]
}

// MailHeader is one header of an attached message, in message order.
export interface MailHeader {
  name: string
  value: string
}

// MailPart is one attachment of a message, fetched by its url.
export interface MailPart {
  index: number
  name: string
  content_type: string
  content_id?: string
  size: number
  url: string
}

// MailMessage is an attached .eml as the viewer shows it. date is
// missing when the message carries none.
export interface MailMessage {
  from: string
  from_name: string
  to: string[]
  cc: string[]
  date?: string
  subject: string
  message_id: string
  received: string[]
  headers: MailHeader[]
  text: string
  html: string
  attachments: MailPart[]
}

// AttachedFile is a file on an entity, url serves it same-origin.
export interface AttachedFile {
  id: string
  sketch_id: string
  node_id: string
  name: string
  content_type: string
  size: number
  hash: string
  uploader_id: string
  created_at: string
  url: string
  is_image: boolean

  // What the file says about itself, read at upload: a camera, a
  // position, an author, dates. Null until read, empty when nothing.
  metadata: Record<string, unknown> | null
}

export interface ConsoleSettings {
  map_style_light: string
  map_style_dark: string
}

// LlmModel is a language model as a person picking it sees it.
// needs_key: the person stores key_name in Secrets to use it.
export interface LlmModel {
  id: string
  name: string
  model: string
  default: boolean
  key_name: string
  needs_key: boolean
}

// LlmModelAdmin is a model as the admin panel sees it, has_api_key and
// never the key.
export interface LlmModelAdmin {
  id: string
  name: string
  api: 'openai' | 'anthropic'
  base_url: string
  model: string
  tools: 'auto' | 'on' | 'off'
  key_name: string
  has_api_key: boolean
  default: boolean
  local: boolean
  created_at: string
  updated_at: string
}

// LlmModelInput creates or patches a model. api_key absent keeps the
// stored one, empty clears it.
export interface LlmModelInput {
  name?: string
  api?: string
  base_url?: string
  model?: string
  tools?: string
  key_name?: string
  api_key?: string
  default?: boolean
}

// MailSettings is the relay as the admin panel sees it.
export interface MailSettings {
  host: string
  port: number
  username: string
  has_password: boolean
  from: string
  starttls: boolean
  enabled: boolean
}

// WorkerSettings is the worker's tuning, the durations in seconds.
export interface WorkerSettings {
  enabled: boolean
  // Empty keeps downloads in the default directory.
  cache_dir: string
  concurrency: number
  poll_seconds: number
  lease_seconds: number
  max_attempts: number
}

// ToolSetting is one tool the transforms use. found is where the node
// answering finds its program, empty when it does not, binary empty
// for a tool that is no program.
export interface ToolSetting {
  name: string
  title: string
  description: string
  binary: string
  path: string
  allow_private: boolean
  found: string
}

// FileStore is where the bytes of attached files live, with the
// limits. default_dir is the directory used when none is set, empty on
// postgres.
export interface FileStore {
  backend: 'fs' | 's3' | 'db'
  max_size_mb: number
  retain_hours: number
  dir: string
  default_dir: string
  s3: {
    endpoint: string
    region: string
    bucket: string
    prefix: string
    access_key: string
    has_secret_key: boolean
    path_style: boolean
  }
}

// FileStoreInput saves the file store. s3.secret_key absent keeps the
// stored one.
export interface FileStoreInput {
  backend: string
  max_size_mb: number
  retain_hours: number
  dir: string
  s3: {
    endpoint: string
    region: string
    bucket: string
    prefix: string
    access_key: string
    secret_key?: string
    path_style: boolean
  }
}

// MailSettingsInput saves the relay. password absent keeps the stored
// one.
export interface MailSettingsInput {
  host: string
  port: number
  username: string
  password?: string
  from: string
  starttls: boolean
}

export interface Analysis {
  id: string
  investigation_id: string
  owner_id: string
  title: string
  content: unknown
  created_at: string
  updated_at: string
}

export interface Chat {
  id: string
  sketch_id: string
  owner_id: string
  title: string

  // entities are the ids every question of the chat was asked about.
  entities: string[]
  created_at: string
  updated_at: string
}

export interface ChatMessage {
  id: string
  chat_id: string
  role: string
  content: string
  context: string[]
  created_at: string
}

// ProposedChange is one edit the assistant suggests for the graph, or
// a run it asks the person to start: one transform with params, or a
// flow of steps from an input type, on node_ids.
export interface ProposedChange {
  kind: 'set_properties' | 'add_entity' | 'link' | 'run'
  node_id?: string
  properties?: Record<string, unknown>
  type?: string
  link_to?: string
  relation?: string
  source?: string
  target?: string
  label?: string
  reason?: string
  transform?: string
  params?: Record<string, string>
  node_ids?: string[]
  input?: string
  steps?: FlowStep[]
}

export interface ApplyResult {
  applied: number
  launched: number
  nodes: GraphNode[]
  errors: string[]
}
