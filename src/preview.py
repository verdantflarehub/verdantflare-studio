"""Authenticated Studio Market preview; consumes the Station Core contract."""
from __future__ import annotations
import os
import re
import secrets
from pathlib import Path
import httpx
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import FileResponse, JSONResponse, Response
from starlette.routing import Route, Mount
from starlette.staticfiles import StaticFiles

ROOT = Path(__file__).parent / 'static' / 'market'
COOKIE = 'vf_station_session'
CORE = os.environ.get('STATION_CORE_URL', 'http://station-core:5050').rstrip('/')
ORIGIN = os.environ.get('STUDIO_PUBLIC_ORIGIN', 'https://mcp.cn-chengdu.bc-cloud.com').rstrip('/')

async def page(request: Request):
    return FileResponse(ROOT/'index.html',headers={'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'})

async def health(request: Request):
    return JSONResponse({'status':'ok','service':'verdantflare-studio','version':'0.1.1'})

async def api(request: Request):
    route=request.path_params['path']
    method=request.method
    login=route=='login' and method=='POST'
    logout=route=='logout' and method=='POST'
    command=route=='commands' and method=='POST'
    read=method=='GET' and (route in ('me','health','apps') or re.fullmatch(r'apps/[a-z0-9][a-z0-9-]{0,62}',route))
    if not (login or logout or command or read):return JSONResponse({'code':'NOT_FOUND','message':'接口尚未开放'},404)
    if method=='POST' and request.headers.get('origin')!=ORIGIN:
        return JSONResponse({'code':'PERMISSION_DENIED','message':'请求来源不匹配'},403)
    token=request.cookies.get(COOKIE,'')
    if not login and not token:return JSONResponse({'code':'UNAUTHENTICATED','message':'请先登录'},401)
    headers={'X-Request-ID':secrets.token_hex(16)}
    if token:headers['Authorization']='Bearer '+token
    body=None
    if login:
        if request.headers.get('content-type','').split(';')[0]!='application/json':return JSONResponse({'code':'INVALID_ARGUMENT'},400)
        chunks=[];size=0
        async for chunk in request.stream():
            size+=len(chunk)
            if size>16384:return JSONResponse({'code':'INVALID_ARGUMENT'},400)
            chunks.append(chunk)
        body=b''.join(chunks);headers['Content-Type']='application/json'
    elif logout: body=b'{}';headers['Content-Type']='application/json'
    elif command:
        if request.headers.get('content-type','').split(';')[0]!='application/json': return JSONResponse({'code':'INVALID_ARGUMENT'},400)
        body=await request.body();
        if len(body)>16384:return JSONResponse({'code':'INVALID_ARGUMENT'},400)
        headers['Content-Type']='application/json'
    paths={'login':'/identity/login','logout':'/identity/logout','commands':'/app-commands','me':'/identity/me','health':'/station/health','apps':'/catalog/apps'}
    path=paths.get(route,'/catalog/'+route)
    try:
        async with httpx.AsyncClient(timeout=15,follow_redirects=False) as client:
            result=await client.request(method,CORE+path,headers=headers,content=body,params=list(request.query_params.multi_items()))
        if result.status_code==204:
            response=Response(status_code=204)
        else:
            data=result.json()
            if not isinstance(data,dict):raise ValueError('invalid upstream response')
            access=data.pop('access_token',None)
            response=JSONResponse(data,status_code=result.status_code)
            if login and result.status_code==201:
                if not isinstance(access,str) or not re.fullmatch(r'[A-Za-z0-9_-]{43}',access):raise ValueError('invalid session')
                response.set_cookie(COOKIE,access,httponly=True,secure=True,samesite='strict',path='/studio')
        if logout or result.status_code==401:
            response.delete_cookie(COOKIE,path='/studio',secure=True,httponly=True,samesite='strict')
        response.headers['Cache-Control']='no-store'
        response.headers['X-Content-Type-Options']='nosniff'
        response.headers['X-Request-ID']=headers['X-Request-ID']
        return response
    except (httpx.HTTPError,ValueError):
        return JSONResponse({'code':'SERVICE_UNAVAILABLE','message':'Station 暂时无法连接，请稍后刷新'},503,headers={'Cache-Control':'no-store'})

app=Starlette(routes=[Route('/health',health),Route('/studio',page),Route('/studio/',page),Route('/studio/api/{path:path}',api,methods=['GET','POST']),Mount('/studio/static',app=StaticFiles(directory=ROOT))])
