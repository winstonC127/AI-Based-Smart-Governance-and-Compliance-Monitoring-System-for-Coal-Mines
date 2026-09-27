/**
 * api.js — thin fetch wrapper around the Go backend REST API.
 * Every response from the backend follows { success, message, data, error }.
 * This module normalizes that so callers can just `await api.get(...)`.
 */
const API = (() => {
  const CACHE_TTL_MS = 45 * 1000; // 45 seconds cache for static/reference lookups
  const cache = new Map();
  const inFlight = new Map();

  const CACHEABLE_ENDPOINTS = [
    '/mines',
    '/categories',
    '/subsidiaries',
    '/users/assignable',
    '/compliance-rules',
    '/contractors'
  ];

  function getBaseUrl() {
    const raw = window.APP_CONFIG?.API_BASE_URL || 'http://localhost:8080/api';
    return raw.replace(/\/+$/, '');
  }

  function getToken() {
    return localStorage.getItem('cg_token');
  }

  function shouldCache(method, cleanPath) {
    if (method !== 'GET') return false;
    return CACHEABLE_ENDPOINTS.some(ep => cleanPath === ep || cleanPath.startsWith(ep + '?'));
  }

  function invalidateCacheFor(path) {
    for (const key of cache.keys()) {
      if (path.includes(key) || key.includes(path.split('/')[1] || '')) {
        cache.delete(key);
      }
    }
  }

  async function request(method, path, body, options = {}) {
    const cleanPath = path.startsWith('/') ? path : '/' + path;

    // Cache check for GET requests
    if (shouldCache(method, cleanPath) && !options.noCache) {
      const cached = cache.get(cleanPath);
      if (cached && (Date.now() - cached.timestamp < CACHE_TTL_MS)) {
        return cached.data;
      }
      // In-flight deduplication
      if (inFlight.has(cleanPath)) {
        return inFlight.get(cleanPath);
      }
    }

    const fetchPromise = (async () => {
      const headers = { 'Content-Type': 'application/json' };
      const token = getToken();
      if (token) headers['Authorization'] = `Bearer ${token}`;

      const baseUrl = getBaseUrl();
      const url = `${baseUrl}${cleanPath}`;

      // 15 second request timeout via AbortController
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 15000);

      let res;
      try {
        res = await fetch(url, {
          method,
          headers,
          body: body ? JSON.stringify(body) : undefined,
          signal: controller.signal,
        });
      } catch (networkErr) {
        clearTimeout(timeoutId);
        if (networkErr.name === 'AbortError') {
          throw new Error('Server request timed out. Please check your network connection.');
        }
        throw new Error('Cannot reach the server. Check that the backend is running.');
      } finally {
        clearTimeout(timeoutId);
      }

      let json;
      try {
        json = await res.json();
      } catch (parseErr) {
        throw new Error(`Unexpected server response (status ${res.status}).`);
      }

      if (res.status === 401) {
        // Session expired or invalid — force re-login.
        localStorage.removeItem('cg_token');
        localStorage.removeItem('cg_user');
        if (!location.pathname.endsWith('login.html')) {
          location.href = 'login.html?expired=1';
        }
        throw new Error(json.message || 'Session expired');
      }

      if (!json.success) {
        throw new Error(json.message || 'Request failed');
      }

      // If mutation, invalidate related cached GET endpoints
      if (method !== 'GET') {
        invalidateCacheFor(cleanPath);
      } else if (shouldCache(method, cleanPath)) {
        cache.set(cleanPath, { data: json.data, timestamp: Date.now() });
      }

      return json.data;
    })();

    if (shouldCache(method, cleanPath)) {
      inFlight.set(cleanPath, fetchPromise);
      try {
        const data = await fetchPromise;
        return data;
      } finally {
        inFlight.delete(cleanPath);
      }
    }

    return fetchPromise;
  }

  return {
    get: (path, options) => request('GET', path, undefined, options),
    post: (path, body, options) => request('POST', path, body, options),
    put: (path, body, options) => request('PUT', path, body, options),
    delete: (path, body, options) => request('DELETE', path, body, options),
    del: (path, body, options) => request('DELETE', path, body, options),
    clearCache: () => cache.clear()
  };
})();

