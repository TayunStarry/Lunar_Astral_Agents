// ============================================================
//  星月智能 · 消息终端 — 标签页过滤与全文搜索
// ============================================================

// ---------- 标签页过滤 ----------
function setupTabEvents() {
    tabBar.addEventListener('click', (e) => {
        const btn = e.target.closest('.tab');
        if (!btn) return;
        tabBar.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        btn.classList.add('active');
        currentTab = btn.dataset.tab;
        applyFilters();
    });
}

// ---------- 搜索 ----------
function clearHighlights() {
    document.querySelectorAll('.search-highlight').forEach(mark => {
        const parent = mark.parentNode;
        if (parent) {
            parent.replaceChild(document.createTextNode(mark.textContent), mark);
            parent.normalize();
        }
    });
}

function highlightTextInNode(root, query) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
        acceptNode(node) {
            if (!node.nodeValue || !node.nodeValue.toLowerCase().includes(query)) return NodeFilter.FILTER_REJECT;
            const parent = node.parentElement;
            if (!parent || parent.tagName === 'SCRIPT' || parent.tagName === 'STYLE' || parent.classList.contains('search-highlight')) return NodeFilter.FILTER_REJECT;
            return NodeFilter.FILTER_ACCEPT;
        }
    });
    const textNodes = [];
    while (walker.nextNode()) textNodes.push(walker.currentNode);

    textNodes.forEach(node => {
        const text = node.nodeValue;
        const lower = text.toLowerCase();
        const qlen = query.length;
        const frag = document.createDocumentFragment();
        let i = 0;
        let idx;
        while ((idx = lower.indexOf(query, i)) !== -1) {
            if (idx > i) frag.appendChild(document.createTextNode(text.slice(i, idx)));
            const mark = document.createElement('span');
            mark.className = 'search-highlight';
            mark.textContent = text.slice(idx, idx + qlen);
            frag.appendChild(mark);
            i = idx + qlen;
        }
        if (i < text.length) frag.appendChild(document.createTextNode(text.slice(i)));
        node.parentNode.replaceChild(frag, node);
    });
}

function applyFilters() {
    clearHighlights();
    searchMatches = [];
    currentMatchIndex = -1;

    const q = searchQuery;
    messageArea.querySelectorAll('.message').forEach(el => {
        const cats = (el.dataset.categories || '').split(',');
        const tabOk = currentTab === 'all' || cats.includes(currentTab);
        const searchOk = !q || (el.dataset.searchText || '').includes(q);
        el.style.display = (tabOk && searchOk) ? '' : 'none';
    });

    if (q) {
        messageArea.querySelectorAll('.message').forEach(el => {
            if (el.style.display === 'none') return;
            const targets = el.querySelectorAll('.markdown-content, .music-abc-preview, .labeled-image-container');
            targets.forEach(t => highlightTextInNode(t, q));
        });
        searchMatches = Array.from(document.querySelectorAll('.search-highlight'));
    }

    updateSearchUI();
}

function updateSearchUI() {
    const q = searchQuery;
    searchClear.hidden = !q;
    searchPrev.hidden = !q || searchMatches.length === 0;
    searchNext.hidden = !q || searchMatches.length === 0;
    if (q) {
        searchCount.hidden = false;
        searchCount.textContent = searchMatches.length ? `${currentMatchIndex + 1}/${searchMatches.length}` : '0/0';
    } else {
        searchCount.hidden = true;
    }
}

function goToMatch(delta) {
    if (!searchMatches.length) return;
    currentMatchIndex = (currentMatchIndex + delta + searchMatches.length) % searchMatches.length;
    searchMatches.forEach(m => m.classList.remove('current'));
    const current = searchMatches[currentMatchIndex];
    current.classList.add('current');
    current.scrollIntoView({ block: 'center', behavior: 'smooth' });
    updateSearchUI();
}

function setupSearchEvents() {
    searchInput.addEventListener('input', () => {
        searchQuery = searchInput.value.trim().toLowerCase();
        currentMatchIndex = -1;
        applyFilters();
    });
    searchClear.addEventListener('click', () => {
        searchInput.value = '';
        searchQuery = '';
        applyFilters();
        searchInput.focus();
    });
    searchPrev.addEventListener('click', () => goToMatch(-1));
    searchNext.addEventListener('click', () => goToMatch(1));
}
