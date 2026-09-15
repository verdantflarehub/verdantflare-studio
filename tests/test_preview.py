import secrets
import unittest
from unittest.mock import patch
import httpx
from starlette.testclient import TestClient
from src import preview

TOKEN=secrets.token_urlsafe(32)
class Upstream:
    calls=[]
    async def __aenter__(self):return self
    async def __aexit__(self,*args):pass
    async def request(self,method,url,**kwargs):
        self.calls.append((method,url,kwargs))
        if url.endswith('/identity/login'):
            return httpx.Response(201,json={'access_token':TOKEN,'username':'admin','expires_at':'2026-09-15T23:00:00Z'})
        if url.endswith('/identity/logout'):return httpx.Response(204)
        return httpx.Response(200,json={'items':[],'next_cursor':None})

class PreviewTests(unittest.TestCase):
    def setUp(self):
        self.client=TestClient(preview.app,base_url=preview.ORIGIN)
        self.mock=patch.object(preview.httpx,'AsyncClient',return_value=Upstream());self.mock.start();self.addCleanup(self.mock.stop)
    def test_session_cookie_and_catalog(self):
        self.assertEqual(self.client.get('/studio/api/apps').status_code,401)
        r=self.client.post('/studio/api/login',headers={'Origin':preview.ORIGIN},json={'username':'admin','password':secrets.token_urlsafe(20)})
        self.assertEqual(r.status_code,201)
        self.assertNotIn(TOKEN,r.text)
        self.assertIn('HttpOnly',r.headers['set-cookie']);self.assertIn('Secure',r.headers['set-cookie']);self.assertIn('SameSite=strict',r.headers['set-cookie'])
        self.assertEqual(self.client.get('/studio/api/apps?group_id=video').status_code,200)
        self.assertEqual(Upstream.calls[-1][1],preview.CORE+'/catalog/apps')
        self.assertEqual(Upstream.calls[-1][2]['headers']['Authorization'],'Bearer '+TOKEN)
        self.assertEqual(self.client.post('/studio/api/logout',headers={'Origin':preview.ORIGIN}).status_code,204)
        self.assertEqual(self.client.get('/studio/api/apps').status_code,401)
    def test_origin_and_proxy_boundary(self):
        self.assertEqual(self.client.post('/studio/api/login',headers={'Origin':'https://other.invalid'},json={}).status_code,403)
        self.assertEqual(self.client.get('/studio/api/secrets').status_code,404)
        self.assertEqual(self.client.post('/studio/api/apps').status_code,404)
        self.assertEqual(self.client.get('/api/projects').status_code,404)
        self.assertEqual(self.client.get('/studio/').status_code,200)
        self.assertEqual(self.client.get('/studio/static/market.js').status_code,200)
    def test_upstream_unavailable(self):
        self.client.cookies.set(preview.COOKIE,TOKEN,path='/studio')
        with patch.object(Upstream,'request',side_effect=httpx.ConnectError('private upstream details')):
            r=self.client.get('/studio/api/apps')
        self.assertEqual(r.status_code,503)
        self.assertNotIn('private upstream details',r.text)

if __name__=='__main__':unittest.main()
