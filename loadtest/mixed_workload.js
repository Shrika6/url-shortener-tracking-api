import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const MIXED_RPS = Number(__ENV.MIXED_RPS || 200);
const SPOOF_IPS = (__ENV.SPOOF_IPS || 'true').toLowerCase() === 'true';

export const options = {
  scenarios: {
    mixed_80_20: {
      executor: 'constant-arrival-rate',
      rate: MIXED_RPS,
      timeUnit: '1s',
      duration: __ENV.DURATION || '1m',
      preAllocatedVUs: Number(__ENV.PRE_ALLOCATED_VUS || 100),
      maxVUs: Number(__ENV.MAX_VUS || 500),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.02'],
    http_req_duration: ['p(95)<750', 'p(99)<1500'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(95)', 'p(99)'],
};

export function setup() {
  const redirectCodes = [];

  for (let i = 0; i < 25; i++) {
    const payload = JSON.stringify({
      url: `https://example.com/mixed-seed-${i}-${Date.now()}`,
    });

    const res = http.post(`${BASE_URL}/shorten`, payload, {
      headers: buildHeaders(true),
    });

    check(res, {
      'seed shorten status is 201': (r) => r.status === 201,
    });

    if (res.status === 201) {
      redirectCodes.push(res.json().short_code);
    }
  }

  if (redirectCodes.length === 0) {
    throw new Error('setup failed: no seed short codes created');
  }

  return { redirectCodes };
}

export default function (data) {
  const isRedirect = Math.random() < 0.8;

  if (isRedirect) {
    const idx = Math.floor(Math.random() * data.redirectCodes.length);
    const code = data.redirectCodes[idx];

    const res = http.get(`${BASE_URL}/${code}`, {
      redirects: 0,
      headers: buildHeaders(false),
    });

    check(res, {
      'redirect status is 302': (r) => r.status === 302,
    });
    return;
  }

  const unique = `${Date.now()}-${__VU}-${__ITER}`;
  const payload = JSON.stringify({
    url: `https://example.com/mixed-new-${unique}`,
  });

  const res = http.post(`${BASE_URL}/shorten`, payload, {
    headers: buildHeaders(true),
  });

  check(res, {
    'shorten status is 201': (r) => r.status === 201,
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
