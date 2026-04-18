import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const REDIRECT_RPS = Number(__ENV.REDIRECT_RPS || 500);
const SPOOF_IPS = (__ENV.SPOOF_IPS || 'true').toLowerCase() === 'true';

export const options = {
  scenarios: {
    redirect_high_qps: {
      executor: 'constant-arrival-rate',
      rate: REDIRECT_RPS,
      timeUnit: '1s',
      duration: __ENV.DURATION || '1m',
      preAllocatedVUs: Number(__ENV.PRE_ALLOCATED_VUS || 200),
      maxVUs: Number(__ENV.MAX_VUS || 1000),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(95)', 'p(99)'],
};

export function setup() {
  if (__ENV.SHORT_CODE) {
    return { shortCode: __ENV.SHORT_CODE };
  }

  const payload = JSON.stringify({
    url: `https://example.com/loadtest-redirect-${Date.now()}`,
  });

  const res = http.post(`${BASE_URL}/shorten`, payload, {
    headers: buildHeaders(true),
  });

  check(res, {
    'setup shorten status is 201': (r) => r.status === 201,
  });

  if (res.status !== 201) {
    throw new Error(`setup failed: expected 201 from /shorten, got ${res.status}`);
  }

  const body = res.json();
  return { shortCode: body.short_code };
}

export default function (data) {
  const res = http.get(`${BASE_URL}/${data.shortCode}`, {
    redirects: 0,
    headers: buildHeaders(false),
  });

  check(res, {
    'redirect status is 302': (r) => r.status === 302,
  });
}

function buildHeaders(includeContentType) {
  const headers = {};
  if (includeContentType) {
    headers['Content-Type'] = 'application/json';
  }
  if (SPOOF_IPS) {
    headers['X-Forwarded-For'] = syntheticIP();
  }
  return headers;
}

function syntheticIP() {
  const vu = typeof __VU !== 'undefined' ? __VU : Math.floor(Math.random() * 250);
  const iter = typeof __ITER !== 'undefined' ? __ITER : Math.floor(Math.random() * 250);
  const a = (vu % 250) + 1;
  const b = (iter % 250) + 1;
  const c = (Math.floor(Math.random() * 250) % 250) + 1;
  return `10.${a}.${b}.${c}`;
}
