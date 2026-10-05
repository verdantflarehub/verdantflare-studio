export type MockProxyStatus = 'active' | 'pending' | 'off' | 'testing'

export interface MockProxyRow {
  id: string
  name: string
  protocol: string
  host: string
  port: string
  region: string
  latency: string
  expiry: string
  status: MockProxyStatus
  refs: string[]
  tag: string
  tested: string
  exitIp?: string
  probeTarget?: string
  probeVersion?: string
}

export const mockProxies: MockProxyRow[] = [
  {
    id: 'proxy_01DMIT_LA',
    name: 'dmit.la.usa · 美西高速专线 1',
    protocol: 'HTTP',
    host: 'openclash.openclash.svc.cluster.local',
    port: '1081',
    region: '美国 · 洛杉矶',
    latency: '208 ms',
    expiry: '长期有效',
    status: 'active',
    refs: ['ChatGPT 账号 01 (团队主号 · 独占出口)', 'Claude 3.7 生产主线'],
    tag: 'chatgpt-team-01, dmit',
    tested: '刚刚',
    exitIp: '154.21.84.34',
    probeTarget: 'egress_probe · ipify',
    probeVersion: 'mihomo-v1.19.29'
  },
  {
    id: 'proxy_02WEYLAND_LA',
    name: 'weyland.la.usa · 美西 BGP 专线 2',
    protocol: 'HTTP',
    host: 'openclash.openclash.svc.cluster.local',
    port: '1082',
    region: '美国 · 洛杉矶',
    latency: '206 ms',
    expiry: '长期有效',
    status: 'active',
    refs: [
      'ChatGPT 账号 02 (备用批量 · 专属隔离)',
      'Google Omni / Wan Video 专属通道'
    ],
    tag: 'chatgpt-team-02, weyland',
    tested: '刚刚',
    exitIp: '64.186.238.29',
    probeTarget: 'egress_probe · ipify',
    probeVersion: 'mihomo-v1.19.29'
  }
]

export const mockProbeFixtures: Record<
  string,
  { latency: string; ip: string; region: string }
> = {
  proxy_01DMIT_LA: {
    latency: '208 ms',
    ip: '154.21.84.34',
    region: '美国 · 洛杉矶'
  },
  proxy_02WEYLAND_LA: {
    latency: '206 ms',
    ip: '64.186.238.29',
    region: '美国 · 洛杉矶'
  }
}
