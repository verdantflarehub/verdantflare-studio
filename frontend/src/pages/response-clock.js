// Bind time to one response. Include second precision and the entire request
// duration so transport/body delays never extend the source observation's life.
export function responseClock(date, started, monotonic = () => performance.now()) {
  const server = Date.parse(date ?? '');
  if (!Number.isFinite(server) || new Date(server).toUTCString() !== date) {
    throw Error('无法校验服务端时间，请刷新重试。');
  }
  return () => server + 1000 + Math.max(0, monotonic() - started);
}
