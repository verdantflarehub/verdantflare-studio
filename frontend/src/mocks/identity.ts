export interface MockIdentity {
  user_id: string
  username: string
  organization_id: string
  organization_name: string
  station_id: string
  roles: string[]
  scopes: string[]
}

export const mockIdentity: MockIdentity = {
  user_id: '0192f3d4-1111-7aaa-8bbb-1234567890ab',
  username: 'admin',
  organization_id: '0192f3d4-2222-7aaa-8bbb-1234567890ab',
  organization_name: '青岚创意工作室',
  station_id: 'station_dev_5090',
  roles: ['admin', 'creator'],
  scopes: ['*']
}
