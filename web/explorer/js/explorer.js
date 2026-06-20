// ============================================================
// NogoCore Block Explorer — Production JavaScript
// Single-page application with client-side routing, API
// integration, and real-time polling updates.
// ============================================================

(function () {
  'use strict';

  // ---- Debug logger (visible on page) ----
  function debugLog(msg) {
    var el = document.getElementById('debugBar');
    if (el) { el.textContent = '[Explorer] ' + msg; }
    try { console.log('[Explorer]', msg); } catch(e) {}
  }

  // ---- Constants ----
  var API_BASE = '/api/v1';
  var REFRESH_INTERVAL = 15000;

  // ---- State ----
  var state = {
    currentView: 'dashboard',
    bestHeight: 0,
    blocksPage: 1,
    blocksPerPage: 25,
    refreshTimer: null,
    searchTimer: null,
  };

  // ---- DOM helpers ----
  function $(sel, ctx) { return (ctx || document).querySelector(sel); }
  function $$(sel, ctx) { return Array.prototype.slice.call((ctx || document).querySelectorAll(sel)); }

  // ---- Safe DOM update helpers ----
  function safeText(id, value) {
    var el = document.getElementById(id);
    if (el) el.textContent = value;
  }

  function safeHtml(id, html) {
    var el = document.getElementById(id);
    if (el) el.innerHTML = html;
  }

  function safeShow(id) {
    var el = document.getElementById(id);
    if (el) el.hidden = false;
  }

  function safeHide(id) {
    var el = document.getElementById(id);
    if (el) el.hidden = true;
  }

  function safeDisabled(id, val) {
    var el = document.getElementById(id);
    if (el) el.disabled = val;
  }

  // ---- View Management ----
  function showView(name) {
    if (state.currentView === name) return;
    debugLog('showView: ' + name);
    var views = $$('.view');
    for (var i = 0; i < views.length; i++) { views[i].classList.remove('active'); }
    var target = $('#view-' + name);
    if (target) target.classList.add('active');
    var links = $$('.nav-link');
    for (var j = 0; j < links.length; j++) { links[j].classList.remove('active'); }
    var navLink = $('[data-nav="' + name + '"]');
    if (navLink) navLink.classList.add('active');
    state.currentView = name;
    try { window.scrollTo({ top: 0, behavior: 'smooth' }); } catch(e) {}
  }

  function navigate(hash) {
    if (!hash || hash === '#dashboard') { showView('dashboard'); return; }
    if (hash === '#blocks') { showView('blocks'); loadAllBlocks(); return; }
    if (hash === '#mempool') { showView('mempool'); loadMempool(); return; }

    var mBlock = hash.match(/^#block\/(.+)/);
    if (mBlock) { showView('block'); loadBlockDetail(mBlock[1]); return; }

    var mTx = hash.match(/^#tx\/(.+)/);
    if (mTx) { showView('tx'); loadTxDetail(mTx[1]); return; }

    var mAddr = hash.match(/^#address\/(.+)/);
    if (mAddr) { showView('address'); loadAddressDetail(mAddr[1]); return; }

    showView('error');
    safeText('errorMessage', 'Unknown route: ' + hash);
  }

  // ---- API Helpers ----
  function apiFetch(path) {
    debugLog('fetch: ' + path);
    return fetch(API_BASE + path)
      .then(function(resp) {
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        return resp.json();
      })
      .catch(function(err) {
        debugLog('ERROR: ' + path + ' - ' + err.message);
        return null;
      });
  }

  // ---- Copy to clipboard (cross-browser) ----
  function copyToClipboard(text, el, originalText) {
    // Try modern Clipboard API first
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(function() {
        if (el) { el.textContent = '\u2713'; setTimeout(function() { el.textContent = originalText; }, 1500); }
      }).catch(function() {
        fallbackCopy(text, el, originalText);
      });
      return;
    }
    fallbackCopy(text, el, originalText);
  }

  function fallbackCopy(text, el, originalText) {
    var textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.left = '-9999px';
    textarea.style.top = '-9999px';
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    try {
      var ok = document.execCommand('copy');
      if (ok && el) {
        el.textContent = '\u2713';
        setTimeout(function() { el.textContent = originalText; }, 1500);
      }
    } catch(e) {
      debugLog('Copy failed: ' + e.message);
    }
    document.body.removeChild(textarea);
  }

  // ---- Formatting Helpers ----
  function truncateHash(hash, len) {
    if (!hash || hash.length <= len * 2 + 3) return hash || '--';
    return hash.slice(0, len) + '...' + hash.slice(-len);
  }

  function formatTimestamp(ts) {
    if (!ts) return '--';
    var d = new Date(ts * 1000);
    var now = new Date();
    var diffSec = Math.floor((now - d) / 1000);
    if (diffSec < 60) return diffSec + 's ago';
    if (diffSec < 3600) return Math.floor(diffSec / 60) + 'm ago';
    if (diffSec < 86400) return Math.floor(diffSec / 3600) + 'h ago';
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  function formatNumber(n) {
    if (n === null || n === undefined) return '--';
    if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B';
    if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M';
    if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
    return n.toLocaleString();
  }

  function formatBytes(bytes) {
    if (!bytes) return '--';
    if (bytes >= 1048576) return (bytes / 1048576).toFixed(2) + ' MB';
    if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return bytes + ' B';
  }

  function formatNOGO(atomic) {
    if (!atomic) return '--';
    var nogo = parseFloat(atomic) / 1e8;
    return nogo.toLocaleString('en-US', { minimumFractionDigits: 8, maximumFractionDigits: 8 }) + ' NOGO';
  }

  // ---- Dashboard ----
  function loadDashboardStats() {
    debugLog('loading dashboard stats...');
    apiFetch('/stats').then(function(stats) {
      if (!stats) {
        debugLog('stats fetch failed!');
        return;
      }
      debugLog('stats OK, height=' + stats.block_height);

      safeText('statHeight', formatNumber(stats.block_height));
      updateStatCopy('statBestHash', stats.best_hash, 10);
      safeText('statDifficulty', formatNumber(stats.difficulty));
      safeText('statMempool', formatNumber(stats.mempool_size));
      safeText('statBlockTime', (stats.block_time_seconds || 60) + 's');
      safeText('statHashrate', formatNumber(stats.estimated_hashrate || 0) + ' H/s');

      state.bestHeight = stats.block_height ? Number(stats.block_height) : 0;
      safeText('footerStats', 'Block ' + formatNumber(stats.block_height) + ' | ' + (stats.network || 'mainnet'));

      loadRecentBlocks();
    });
  }

  function updateStatCopy(id, fullHash, len) {
    var el = document.getElementById(id);
    if (!el) return;
    var display = truncateHash(fullHash, len);
    el.textContent = display;
    el.title = 'Click to copy: ' + fullHash;
    el.style.cursor = 'pointer';
    el.onclick = function() {
      copyToClipboard(fullHash, this, display);
    };
  }

  function loadRecentBlocks() {
    var tbody = document.getElementById('recentBlocksBody');
    if (!tbody) {
      debugLog('recentBlocksBody NOT FOUND!');
      return;
    }
    debugLog('loadRecentBlocks, bestHeight=' + state.bestHeight);

    var endHeight = state.bestHeight;
    var startHeight = Math.max(0, endHeight - 9);
    debugLog('fetching blocks ' + startHeight + ' to ' + endHeight);

    var blocks = [];
    var promises = [];
    for (var h = endHeight; h >= startHeight && h >= 0; h--) {
      (function(height) {
        promises.push(
          apiFetch('/block/' + height).then(function(b) {
            if (b && !b.error) {
              blocks.push(b);
              debugLog('got block ' + height);
            } else {
              debugLog('block ' + height + ' invalid: ' + (b ? JSON.stringify(b).substring(0,100) : 'null'));
            }
          })
        );
      })(h);
    }

    Promise.all(promises).then(function() {
      debugLog('blocks fetched: ' + blocks.length);
      blocks.sort(function(a, b) { return b.height - a.height; });

      if (blocks.length === 0) {
        tbody.innerHTML = '<tr class="table-empty"><td colspan="5">No blocks yet</td></tr>';
        return;
      }

      var html = '';
      for (var i = 0; i < Math.min(blocks.length, 10); i++) {
        var b = blocks[i];
        var hashShort = truncateHash(b.hash, 8);
        html += '<tr>' +
          '<td><a href="#block/' + b.height + '" class="highlight">#' + formatNumber(b.height) + '</a></td>' +
          '<td class="hash-cell"><a href="#block/' + b.hash + '" class="hash-link">' + hashShort + '</a><button class="copy-btn" data-hash="' + b.hash + '" title="Copy hash">\u2398</button></td>' +
          '<td>' + (b.tx_count || 0) + '</td>' +
          '<td>' + formatBytes(b.size) + '</td>' +
          '<td>' + formatTimestamp(b.timestamp) + '</td>' +
          '</tr>';
      }
      tbody.innerHTML = html;

      // Add click-to-copy for copy buttons
      var copyBtns = tbody.querySelectorAll('.copy-btn');
      for (var k = 0; k < copyBtns.length; k++) {
        copyBtns[k].addEventListener('click', function(e) {
          e.stopPropagation();
          copyToClipboard(this.getAttribute('data-hash'), this, '\u2398');
        });
      }
    });
  }

  // ---- All Blocks ----
  function loadAllBlocks() {
    var tbody = document.getElementById('allBlocksBody');
    if (!tbody) return;

    apiFetch('/stats').then(function(stats) {
      if (!stats) return;
      state.bestHeight = Number(stats.block_height) || 0;

      var endHeight = state.bestHeight;
      var startHeight = Math.max(0, endHeight - state.blocksPerPage + 1);

      var promises = [];
      for (var h = endHeight; h >= startHeight && h >= 0; h--) {
        (function(height) {
          promises.push(
            apiFetch('/block/' + height).then(function(b) {
              if (b && !b.error) return b;
              return null;
            })
          );
        })(h);
      }

      Promise.all(promises).then(function(results) {
        var blocks = results.filter(function(b) { return b !== null; });
        blocks.sort(function(a, b) { return b.height - a.height; });

        if (blocks.length === 0) {
          tbody.innerHTML = '<tr class="table-empty"><td colspan="5">No blocks yet</td></tr>';
          return;
        }

        var html = '';
        for (var i = 0; i < blocks.length; i++) {
          var b = blocks[i];
          var hashShort = truncateHash(b.hash, 10);
          var ts = b.timestamp ? new Date(b.timestamp * 1000).toLocaleString() : '--';
          html += '<tr>' +
            '<td><a href="#block/' + b.height + '" class="highlight">#' + formatNumber(b.height) + '</a></td>' +
            '<td class="hash-cell"><a href="#block/' + b.hash + '" class="hash-link">' + hashShort + '</a><button class="copy-btn" data-hash="' + b.hash + '" title="Copy hash">\u2398</button></td>' +
            '<td>' + (b.tx_count || 0) + '</td>' +
            '<td>' + formatBytes(b.size) + '</td>' +
            '<td>' + ts + '</td>' +
            '</tr>';
        }
        tbody.innerHTML = html;

        // Add click-to-copy for copy buttons
        var copyBtns = tbody.querySelectorAll('.copy-btn');
        for (var k = 0; k < copyBtns.length; k++) {
          copyBtns[k].addEventListener('click', function(e) {
            e.stopPropagation();
            copyToClipboard(this.getAttribute('data-hash'), this, '\u2398');
          });
        }

        updatePagination();
      });
    });
  }

  function updatePagination() {
    var totalPages = Math.max(1, Math.ceil((state.bestHeight + 1) / state.blocksPerPage));
    safeText('blocksPageInfo', 'Page ' + state.blocksPage + ' of ' + totalPages);
    safeDisabled('blocksPrev', state.blocksPage <= 1);
    safeDisabled('blocksNext', state.blocksPage >= totalPages);
    if (totalPages <= 1) {
      safeHide('blocksPagination');
    } else {
      safeShow('blocksPagination');
    }
  }

  // ---- Block Detail ----
  function loadBlockDetail(identifier) {
    apiFetch('/block/' + identifier).then(function(block) {
      if (!block || block.error) {
        showView('error');
        safeText('errorMessage', 'Block not found: ' + identifier);
        return;
      }

      safeText('blockDetailHeight', '#' + formatNumber(block.height));
      makeCopyValue('bdHash', block.hash || '--', 10);
      makeCopyValue('bdPrevBlock', block.prev_block || '--', 10);
      makeCopyValue('bdMerkleRoot', block.merkle_root || '--', 10);
      safeText('bdTimestamp', block.timestamp ? new Date(block.timestamp * 1000).toLocaleString() : '--');
      safeText('bdVersion', '0x' + (block.version || 0).toString(16));
      safeText('bdBits', '0x' + (block.bits || 0).toString(16));
      safeText('bdNonce', (block.nonce !== undefined ? block.nonce : '--').toString());
      safeText('bdSize', formatBytes(block.size));
      safeText('bdTxCount', formatNumber(block.tx_count));
      safeText('bdDifficulty', block.difficulty || '--');

      var txsBody = document.getElementById('blockTxsBody');
      if (txsBody) {
        if (block.txids && block.txids.length > 0) {
          var txHtml = '';
          for (var i = 0; i < block.txids.length; i++) {
            var txid = block.txids[i];
            var txShort = truncateHash(txid, 12);
            txHtml += '<tr><td>' + i + '</td><td class="hash-cell"><a href="#tx/' + txid + '" class="hash-link">' + txShort + '</a><button class="copy-btn" data-hash="' + txid + '" title="Copy TX hash">\u2398</button></td></tr>';
          }
          txsBody.innerHTML = txHtml;

          var txCopyBtns = txsBody.querySelectorAll('.copy-btn');
          for (var k = 0; k < txCopyBtns.length; k++) {
            txCopyBtns[k].addEventListener('click', function(e) {
              e.stopPropagation();
              copyToClipboard(this.getAttribute('data-hash'), this, '\u2398');
            });
          }
        } else {
          txsBody.innerHTML = '<tr class="table-empty"><td colspan="2">No transactions</td></tr>';
        }
      }
    });
  }

  // Helper: make a detail value clickable to copy
  function makeCopyValue(id, fullValue, len) {
    var el = document.getElementById(id);
    if (!el) return;
    if (!fullValue || fullValue === '--') {
      el.textContent = '--';
      return;
    }
    var display = truncateHash(fullValue, len);
    el.textContent = display;
    el.title = 'Click to copy: ' + fullValue;
    el.style.cursor = 'pointer';
    el.style.color = 'var(--accent)';
    el.onclick = function() {
      copyToClipboard(fullValue, this, display);
    };
  }

  // ---- Transaction Detail ----
  function loadTxDetail(txid) {
    apiFetch('/tx/' + txid).then(function(tx) {
      if (!tx || tx.error) {
        showView('error');
        safeText('errorMessage', 'Transaction not found: ' + txid);
        return;
      }

      safeText('tdTxid', tx.txid || txid);
      safeText('tdSize', formatBytes(tx.size || 0));
      safeHtml('tdStatus', '<span style="color:var(--success);font-weight:600">Confirmed</span>');

      var inputsEl = document.getElementById('txInputsBody');
      if (inputsEl) {
        if (tx.vin && tx.vin.length > 0) {
          var inHtml = '';
          for (var i = 0; i < tx.vin.length; i++) {
            inHtml += '<div class="io-item"><span class="io-label">#' + i + '</span><span class="io-value">' +
              (tx.vin[i].txid ? '<a href="#tx/' + tx.vin[i].txid + '">' + truncateHash(tx.vin[i].txid, 10) + '</a>:' + (tx.vin[i].vout || 0) : 'Coinbase') +
              '</span></div>';
          }
          inputsEl.innerHTML = inHtml;
        } else {
          inputsEl.innerHTML = '<p class="empty-message">No inputs</p>';
        }
      }

      var outputsEl = document.getElementById('txOutputsBody');
      if (outputsEl) {
        if (tx.vout && tx.vout.length > 0) {
          var outHtml = '';
          for (var j = 0; j < tx.vout.length; j++) {
            outHtml += '<div class="io-item"><span class="io-label">#' + j + '</span><span class="io-value">' +
              formatNOGO(tx.vout[j].value) + ' → <span class="io-address">' + truncateHash(tx.vout[j].address || '--', 8) + '</span></span></div>';
          }
          outputsEl.innerHTML = outHtml;
        } else {
          outputsEl.innerHTML = '<p class="empty-message">No outputs</p>';
        }
      }
    });
  }

  // ---- Address Detail ----
  function loadAddressDetail(addr) {
    apiFetch('/address/' + addr).then(function(data) {
      if (!data || data.error) {
        safeText('adAddress', addr);
        safeText('adBalance', '0.00000000 NOGO');
        safeText('adTxCount', '0');
        return;
      }
      safeText('adAddress', data.address || addr);
      safeText('adBalance', formatNOGO(data.balance));
      safeText('adTxCount', formatNumber(data.tx_count || 0));
    });
  }

  // ---- Mempool View ----
  function loadMempool() {
    apiFetch('/stats').then(function(stats) {
      if (!stats || !stats.mempool_size || stats.mempool_size === 0) {
        var tbody = document.getElementById('mempoolTxsBody');
        if (tbody) tbody.innerHTML = '<tr class="table-empty"><td colspan="5">No pending transactions</td></tr>';
        safeText('mempoolCount', '0 txs');
        return;
      }
      safeText('mempoolCount', formatNumber(stats.mempool_size) + ' txs');
      safeText('statMempool', formatNumber(stats.mempool_size));

      // Mempool entries are fetched via polling the stats endpoint.
      // For full mempool listing, the JSON-RPC getrawmempool method
      // provides complete transaction details.
      var tbody = document.getElementById('mempoolTxsBody');
      if (tbody) {
        tbody.innerHTML = '<tr class="table-empty"><td colspan="5">'
          + formatNumber(stats.mempool_size) + ' pending transaction(s) | '
          + formatBytes(stats.mempool_bytes || 0) + ' total</td></tr>';
      }
    });
  }

  // ---- Search ----
    query = query.trim();
    if (!query) return;

    if (/^\d+$/.test(query)) {
      window.location.hash = '#block/' + query;
    } else if (/^[0-9a-fA-F]{64}$/.test(query)) {
      checkAndNavigate(query);
    } else if (query.length >= 26 && query.length <= 42) {
      window.location.hash = '#address/' + query;
    } else {
      showView('error');
      safeText('errorMessage', 'Invalid search query: "' + query + '". Try a block height, hash, tx hash, or address.');
    }
  }

  function updateSearchResults(suggestions) {
    var resultsEl = document.getElementById('searchResults');
    if (!resultsEl) return;
    if (!suggestions || suggestions.length === 0) {
      resultsEl.hidden = true;
      return;
    }
    var html = '';
    for (var i = 0; i < suggestions.length; i++) {
      html += '<a href="' + suggestions[i].url + '" class="search-result-item"><span class="result-type">' + suggestions[i].type + '</span><span>' + suggestions[i].label + '</span></a>';
    }
    resultsEl.innerHTML = html;
    resultsEl.hidden = false;
  }

  function checkAndNavigate(query) {
    apiFetch('/block/' + query).then(function(block) {
      if (block && !block.error) {
        window.location.hash = '#block/' + query;
      } else {
        window.location.hash = '#tx/' + query;
      }
    });
  }

  // ---- Event Listeners ----
  function bindEvents() {
    debugLog('binding events...');

    // Navigation links
    var navLinks = $$('.nav-link');
    for (var i = 0; i < navLinks.length; i++) {
      navLinks[i].addEventListener('click', (function(link) {
        return function(e) {
          e.preventDefault();
          window.location.hash = '#' + link.getAttribute('data-nav');
        };
      })(navLinks[i]));
    }

    // Back buttons
    var backBtns = $$('.back-btn, [data-back]');
    for (var j = 0; j < backBtns.length; j++) {
      backBtns[j].addEventListener('click', (function(btn) {
        return function(e) {
          e.preventDefault();
          var target = btn.getAttribute('data-back');
          window.location.hash = target ? '#' + target : '#dashboard';
        };
      })(backBtns[j]));
    }

    // Search
    var searchInput = document.getElementById('globalSearch');
    var searchBtn = document.getElementById('searchBtn');

    if (searchBtn && searchInput) {
      searchBtn.addEventListener('click', function() { handleSearch(searchInput.value); });
      searchInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') handleSearch(searchInput.value);
      });

      // Live search suggestions (debounced)
      searchInput.addEventListener('input', function() {
        clearTimeout(state.searchTimer);
        var q = searchInput.value.trim();
        if (!q) {
          safeHide('searchResults');
          return;
        }
        state.searchTimer = setTimeout(function() {
          var suggestions = [];
          if (/^\d+$/.test(q)) {
            suggestions.push({ type: 'BLOCK', label: 'Block #' + q, url: '#block/' + q });
          } else if (/^[0-9a-fA-F]{64}$/.test(q)) {
            suggestions.push(
              { type: 'BLOCK', label: 'Block: ' + truncateHash(q, 10), url: '#block/' + q },
              { type: 'TX', label: 'Transaction: ' + truncateHash(q, 10), url: '#tx/' + q }
            );
          } else if (q.length >= 26) {
            suggestions.push({ type: 'ADDRESS', label: truncateHash(q, 10), url: '#address/' + q });
          }
          updateSearchResults(suggestions);
        }, 200);
      });
    }

    // Close search results on outside click
    document.addEventListener('click', function(e) {
      if (!e.target.closest('.nav-search')) {
        safeHide('searchResults');
      }
    });

    // Hash change routing
    window.addEventListener('hashchange', function() {
      navigate(window.location.hash);
    });

    // Pagination
    var prevBtn = document.getElementById('blocksPrev');
    var nextBtn = document.getElementById('blocksNext');
    if (prevBtn) {
      prevBtn.addEventListener('click', function() {
        if (state.blocksPage > 1) {
          state.blocksPage--;
          loadAllBlocks();
        }
      });
    }
    if (nextBtn) {
      nextBtn.addEventListener('click', function() {
        state.blocksPage++;
        loadAllBlocks();
      });
    }

    debugLog('events bound OK');
  }

  // ---- Auto-Refresh ----
  function startAutoRefresh() {
    state.refreshTimer = setInterval(function() {
      if (state.currentView === 'dashboard') {
        loadDashboardStats();
      } else if (state.currentView === 'blocks') {
        loadAllBlocks();
      } else if (state.currentView === 'mempool') {
        loadMempool();
      }
    }, REFRESH_INTERVAL);
  }

  // ---- Initialization ----
  function init() {
    debugLog('init() called, readyState=' + document.readyState);
    try {
      bindEvents();
      loadDashboardStats();
      startAutoRefresh();

      if (window.location.hash) {
        navigate(window.location.hash);
      }
      debugLog('init() complete');
    } catch(err) {
      debugLog('INIT ERROR: ' + err.message);
    }
  }

  // Boot
  if (document.readyState === 'loading') {
    debugLog('DOM loading, waiting for DOMContentLoaded');
    document.addEventListener('DOMContentLoaded', init);
  } else {
    debugLog('DOM ready, calling init()');
    init();
  }
})();
