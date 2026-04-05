#!/usr/bin/env node

const https = require('https');

function braveSearch(query, options = {}) {
  return new Promise((resolve, reject) => {
    const apiKey = process.env.BRAVE_API_KEY;

    if (!apiKey) {
      reject({ error: true, type: 'configuration', message: 'BRAVE_API_KEY environment variable is not set. Get a free key at https://brave.com/search/api/' });
      return;
    }

    const params = new URLSearchParams({ q: query });
    if (options.count) params.set('count', String(options.count));
    if (options.offset) params.set('offset', String(options.offset));
    if (options.country) params.set('country', options.country);
    if (options.safesearch) params.set('safesearch', options.safesearch);

    const url = `https://api.search.brave.com/res/v1/web/search?${params}`;

    const req = https.request(url, {
      method: 'GET',
      headers: {
        'Accept': 'application/json',
        'X-Subscription-Token': apiKey
      }
    }, (res) => {
      let data = '';
      res.on('data', (chunk) => { data += chunk; });
      res.on('end', () => {
        if (res.statusCode === 401) {
          reject({ error: true, message: 'Invalid BRAVE_API_KEY' });
          return;
        }
        if (res.statusCode === 429) {
          reject({ error: true, message: 'Rate limit exceeded' });
          return;
        }
        if (res.statusCode !== 200) {
          reject({ error: true, message: `API returned ${res.statusCode}` });
          return;
        }
        try {
          resolve(JSON.parse(data));
        } catch (e) {
          reject({ error: true, message: 'Failed to parse response' });
        }
      });
    });

    req.on('error', (e) => reject({ error: true, message: e.message }));
    req.end();
  });
}

async function main() {
  let input;
  try {
    input = JSON.parse(await getStdin());
  } catch (e) {
    console.error(JSON.stringify({ error: true, message: 'Invalid JSON input' }));
    process.exit(1);
  }

  if (!input.query) {
    console.error(JSON.stringify({ error: true, message: 'query is required' }));
    process.exit(1);
  }

  try {
    const result = await braveSearch(input.query, input);

    const web = (result.web?.results || []).map(r => ({
      title: r.title,
      url: r.url,
      description: r.description
    }));

    const news = (result.news?.results || []).map(r => ({
      title: r.title,
      url: r.url,
      description: r.description
    }));

    const output = {
      query: result.query?.original || input.query,
      results: web.length,
      web: web
    };
    if (news.length > 0) output.news = news;

    console.log(JSON.stringify(output, null, 2));
  } catch (e) {
    console.error(JSON.stringify(e));
    process.exit(1);
  }
}

function getStdin() {
  return new Promise((resolve) => {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('readable', () => {
      let chunk;
      while ((chunk = process.stdin.read()) !== null) data += chunk;
    });
    process.stdin.on('end', () => resolve(data));
  });
}

main();
