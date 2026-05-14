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
  enabled_cmds: string[]
  agent_url: string
  agent_token: string
  bgp_config: BGPConfig | null
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
  role: string
  created_at: string
  last_login: string | null
}

export interface AuditEntry {
  id: number
  created_at: string
  source_ip: string
  user_id: number | null
  node_id: number | null
  command: string
  params: string
  duration_ms: number
  success: boolean
  error_msg: string
}

export interface CommunityRule {
  id: number
  community: string
  severity: 'reject' | 'warning' | 'info' | 'success'
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
  parsed: PingResult | TracerouteResult | MTRResult | BGPResult | ASPathResult | null
  duration_ms: number
  error_msg: string
  error_code: string
  matched_rules: CommunityRule[]
  as_path_enriched: ASInfo[]
}

export interface PingResult {
  packets_sent: number
  packets_recv: number
  packet_loss: number
  min_rtt: number
  avg_rtt: number
  max_rtt: number
  raw: string
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

export interface MTRHop {
  number: number
  host: string
  loss: number
  sent: number
  recv: number
  last: number
  avg: number
  best: number
  worst: number
  st_dev: number
  as_info: ASInfo | null
}

export interface MTRResult {
  hops: MTRHop[]
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
  status: string
  protocol: string
  age: string
}

export interface BGPResult {
  routes: BGPRoute[]
  raw: string
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
  multihop: boolean
  status: string
  created_at: string
  updated_at: string
}

export interface UpdateStatus {
  current: string
  latest: string
  update_available: boolean
  release_url: string
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
