/**
 * config.js — runtime configuration for CoalGuard AI frontend.
 * Resolves the backend API endpoint dynamically:
 * - Localhost -> http://localhost:8080/api
 * - Production -> https://ai-based-smart-governance-and-compliance-fc8y.onrender.com/api
 * - Ensures '/api' suffix is always present
 * - Automatically purges stale localhost overrides on production domains
 */
(function() {
  const isLocal = window.location.hostname === 'localhost' || 
                  window.location.hostname === '127.0.0.1' || 
                  window.location.hostname === '';
  
  const defaultProdApi = 'https://ai-based-smart-governance-and-compliance-fc8y.onrender.com/api';
  let api = isLocal ? 'http://localhost:8080/api' : defaultProdApi;

  try {
    const urlParams = new URLSearchParams(window.location.search);
    const paramApi = urlParams.get('api');
    if (paramApi) {
      api = paramApi;
      localStorage.setItem('API_BASE_URL', api);
    } else {
      const savedApi = localStorage.getItem('API_BASE_URL');
      if (savedApi) {
        // If running in cloud production, ignore and purge stale localhost overrides
        if (!isLocal && (savedApi.includes('localhost') || savedApi.includes('127.0.0.1'))) {
          localStorage.removeItem('API_BASE_URL');
        } else {
          api = savedApi;
        }
      }
    }
  } catch (e) {}

  // Clean trailing slashes
  api = api.replace(/\/+$/, '');
  // Guarantee /api path is present
  if (!api.endsWith('/api')) {
    api = api + '/api';
  }

  window.APP_CONFIG = {
    API_BASE_URL: api,
  };
})();
