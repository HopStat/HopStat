export interface QuerySubmitMeta {
  queryId: string
  command: string
  bgpActive: boolean
  target: string
  nodeId: number
  nodeName: string
}

export interface Node {
  id: number
  name: string
  description: string
  type: 'standalone' | 'lg_node'
  city: string
  country: string
  lat: number | null
  lon: number | null
  active: boolean
  is_default?: boolean
  enabled_cmds: string[]
  agent_url: string
  agent_token: string
  bgp_config: BGPConfig | null
  bgp_active?: boolean
  created_at: string
  updated_at: string
}

export interface BGPConfig {
  router_id: string
  local_as: number
  peer_as: number
  peer_addr: string
  peer_port: number
  auth_pwd: string
  passive_mode: boolean
  tools_source_ip: string
}

export interface User {
  id: number
  email: string
  created_at: string
  last_login: string | null
}

export interface AuditEntry {
  id: number
  created_at: string
  source_ip: string
  user_id: number | null
  node_id: number | null
  node_name?: string
  command: string
  params: string
  duration_ms: number
  success: boolean
  error_msg: string
}

export interface CommunityRule {
  id: number
  community: string
  severity: 'alert' | 'warning' | 'info' | 'success'
  message_i18n: string
  scope: string
  node_id: number | null
  active: boolean
  created_at: string
  updated_at: string
}

export interface QueryResult {
  id: string
  status: 'pending' | 'running' | 'done' | 'error'
  raw: string
  parsed: PingResult | TracerouteResult | BGPResult | ASPathResult | null
  duration_ms: number
  error_msg: string
  error_code: string
  matched_rules: CommunityRule[]
  as_path: number[]
  as_path_prefix?: string
  as_path_enriched: ASInfo[]
}

export interface PingResult {
  packets_sent?: number
  packets_recv?: number
  packet_loss?: number
  min_rtt?: number
  avg_rtt?: number
  max_rtt?: number
  raw?: string
}

export interface Hop {
  number: number
  ip: string
  host: string
  rtt: number[]
  as_info: ASInfo | null
}

export interface TracerouteResult {
  hops: Hop[]
  raw: string
}

export interface BGPRoute {
  prefix: string
  next_hop: string
  as_path: number[]
  local_pref: number
  med: number
  origin: string
  communities: string[]
  matched_rules?: CommunityRule[]
  status: string
  protocol: string
  age: string
  via_default_route?: boolean
  best?: boolean
  node_name?: string
}

export interface BGPResult {
  routes: BGPRoute[]
  raw: string
  target_as?: ASInfo | null
}

export interface ASPathEntry {
  prefix: string
  as_path: number[]
}

export interface ASPathResult {
  asn: number
  prefixes: ASPathEntry[]
  raw: string
}

export interface ASInfo {
  asn: number
  org_name: string
  short_name: string
  country_code: string
  flag_emoji: string
}

export interface MyIPResult {
  ip: string
  city?: string
  country?: string
  country_code?: string
  country_flag?: string
  latitude?: number
  longitude?: number
  timezone?: string
  asn?: number
  asn_org?: string
}

export interface BGPNeighbor {
  id: number
  node_id: number
  local_as: number
  remote_as: number
  peering_ip: string
  neighbor_ip: string
  ipv6_peering_ip: string
  ipv6_neighbor_ip: string
  multihop: boolean
  peer_type?: 'internal' | 'external'
  default_route_as?: number
  status: string
  prefixes_received?: number
  created_at: string
  updated_at: string
}

export interface BGPConfigSettings {
  local_as: number
  router_id: string
  listen_port: number
  add_path_receive?: boolean
  status: 'not_configured' | 'ready' | 'restart_required'
}

export interface BGPLogEntry {
  timestamp: string
  level: string
  message: string
  address?: string
}

export interface UpdateStatus {
  current: string
  latest: string
  update_available: boolean
  release_url: string
  release_name?: string
  release_notes?: string
  release_versions?: string[]
  self_update_enabled: boolean
  self_update_reason?: string
}

export type ResourceLevel = 'ok' | 'warning' | 'critical'

export interface SystemResource {
  percent: number
  level: ResourceLevel
}

export interface SystemStatus {
  cpu: SystemResource
  memory: SystemResource
  memory_used_bytes: number
  memory_total_bytes: number
  cpu_cores: number
  cpu_load_1: number
  cpu_available: boolean
  collected_at: string
}

export interface SystemAddressOption {
  ip: string
  interface?: string
  link_local?: boolean
}

export interface SystemAddresses {
  ipv4: SystemAddressOption[]
  ipv6: SystemAddressOption[]
}

export interface GeoIPStatus {
  configured: boolean
  enabled: boolean
  update_interval: string
  asn_last_download: string
  city_last_download: string
  last_download: string
  asn_build_date: string
  city_build_date: string
  asn_loaded: boolean
  city_loaded: boolean
}

export type GeoIPResolveSource = 'none' | 'blocks' | 'mmdb' | 'dns'

export interface GeoIPSourceCandidate {
  available: boolean
  matched: boolean
  network?: string
  info?: ASInfo
  error?: string
}

export interface GeoIPLookupCity {
  country_iso?: string
  country?: string
  country_flag?: string
  city?: string
  latitude?: number
  longitude?: number
  time_zone?: string
}

export interface GeoIPLookupReport {
  ip: string
  chosen_source: GeoIPResolveSource
  result?: ASInfo
  blocks: GeoIPSourceCandidate
  mmdb: GeoIPSourceCandidate
  dns: GeoIPSourceCandidate
  city?: GeoIPLookupCity
  asn_index_enriched: boolean
  blocks_csv_loaded: boolean
  blocks_csv_count: number
  asn_db_path?: string
  city_db_path?: string
  cache_note: string
}

export interface BGPSessionStatus {
  neighbor_id: number
  node_id: number
  state: string
  remote_as: number
  neighbor_ip: string
  prefixes_received: number
  uptime: string
}

export interface QuickQuery {
  id: number
  command: string
  name: string
  target: string
  node_id?: number | null
  sort_order: number
  active: boolean
}
