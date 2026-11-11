(() => {
  const { esc, pct, fmtClock, clockClass, remainingMs, winner, score, record } = SR;
  const $ = (sel, el = document) => el.querySelector(sel);

  let state = null;
  let offset = 0;
  let tab = localStorage.getItem('sr.tab') || 'round';
  let viewRound = null; // null = latest round
  let filter = '';
  let customOpen = null; // "round:match" of the match whose custom result form is open
  const draft = { name: '', count: 8, names: [], rounds: '', topCut: 8, minutes: 50, paste: false };

  // Quick result buttons, from player 1's perspective: [p1Wins, p2Wins, draws, label, title]
  const QUICK = [
    [2, 0, 0, '2-0'], [2, 1, 0, '2-1'], [1, 0, 0, '1-0'], null,
    [1, 1, 0, '1-1', 'Draw'], [0, 0, 3, 'ID', 'Intentional draw (0-0-3)'], null,
    [0, 1, 0, '0-1'], [1, 2, 0, '1-2'], [0, 2, 0, '0-2'],
  ];

  // ---------- API ----------
  async function api(method, url, body) {
    const t0 = Date.now();
    let res;
    try {
      res = await fetch(url, {
        method,
        headers: body ? { 'Content-Type': 'application/json' } : {},
        body: body ? JSON.stringify(body) : undefined,
      });
    } catch (e) {
      toast('Cannot reach the server', true);
      throw e;
    }
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      toast(data.error || res.statusText, true);
      throw new Error(data.error || res.statusText);
    }
    if ('serverNow' in data) {
      offset = data.serverNow - (t0 + Date.now()) / 2;
      state = data;
      render();
    }
    return data;
  }

  let toastTimer;
  function toast(msg, isError = false) {
    const el = $('#toast');
    el.textContent = msg;
    el.className = isError ? 'error' : '';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), isError ? 5000 : 2500);
  }

  // ---------- Rendering ----------
  function render() {
    const app = $('#app');
    const active = document.activeElement;
    const focusId = active && active.id;
    const sel = focusId && 'selectionStart' in active ? [active.selectionStart, active.selectionEnd] : null;

    if (!state.tournament) {
      app.innerHTML = renderSetup();
      updateSummary();
    } else {
      app.innerHTML = renderHeader() + renderNav() + `<main>${renderTab()}</main>`;
    }
    tick();

    if (focusId) {
      const el = document.getElementById(focusId);
      if (el) {
        el.focus();
        if (sel && el.setSelectionRange) { try { el.setSelectionRange(sel[0], sel[1]); } catch { /* not a text input */ } }
      }
    }
  }

  function tick() {
    const el = $('#clock');
    if (!el || !state || !state.tournament) return;
    const ms = remainingMs(state.tournament.timer, offset);
    el.textContent = fmtClock(ms);
    el.className = 'clock ' + clockClass(ms);
  }

  function nameOf(id) {
    const p = state.tournament.players.find((x) => x.id === id);
    return p ? p.name : '?';
  }

  function renderHeader() {
    const t = state.tournament;
    const modes = [['pairings', 'Pairings'], ['standings', 'Standings'], ['bracket', 'Top Cut'], ['timer', 'Timer']];
    const running = t.timer.running;
    return `<header>
      <h1>${esc(t.name)}</h1>
      <span class="badge gold">${esc(SR.stageLabel(state))}</span>
      <span class="spacer"></span>
      <div class="timer-widget">
        <span class="clock" id="clock"></span>
        <button data-action="timer" data-op="${running ? 'pause' : 'start'}" class="${running ? '' : 'primary'}">${running ? '⏸ Pause' : '▶ Start'}</button>
        <button data-action="timer" data-op="adjust" data-sec="-60" title="Remove a minute">−1m</button>
        <button data-action="timer" data-op="adjust" data-sec="60" title="Add a minute">+1m</button>
        <button data-action="timer" data-op="reset" title="Reset to the full round length">↺ Reset</button>
      </div>
      <div class="row">
        <span class="muted" style="font-size:13px">Big screen shows</span>
        <div class="seg">${modes.map(([m, l]) => `<button class="${t.display === m ? 'active' : ''}" data-action="display" data-mode="${m}">${l}</button>`).join('')}</div>
        <a href="/display" target="_blank"><button>Open display ↗</button></a>
      </div>
    </header>`;
  }

  function renderNav() {
    const t = state.tournament;
    const tabs = [['round', 'Pairings & Results'], ['standings', 'Standings'], ['players', `Players (${t.players.filter((p) => !p.dropped).length})`]];
    if (t.topCut || t.playoff) tabs.push(['topcut', `Top ${t.playoff ? t.playoff.size : t.topCut}`]);
    tabs.push(['settings', 'Settings']);
    if (!tabs.some(([k]) => k === tab)) tab = 'round';
    return `<nav>
      ${tabs.map(([k, l]) => `<button class="${tab === k ? 'active' : ''}" data-action="tab" data-tab="${k}">${l}</button>`).join('')}
      <span class="spacer"></span>
      <div class="row" style="padding-bottom:6px">
        <input id="announce" placeholder="Big-screen announcement…" value="${esc(t.announcement)}" style="width:280px">
        <button data-action="announce">Show</button>
        ${t.announcement ? '<button data-action="clearAnnounce">Clear</button>' : ''}
      </div>
    </nav>`;
  }

  function renderTab() {
    switch (tab) {
      case 'standings': return renderStandings();
      case 'players': return renderPlayers();
      case 'topcut': return renderTopCut();
      case 'settings': return renderSettings();
      default: return renderRound();
    }
  }

  // ----- Setup -----
  function renderSetup() {
    const rec = SR.recommendedRounds(draft.count);
    const inputs = [];
    for (let i = 0; i < draft.count; i++) {
      inputs.push(`<div class="name-input"><span>${i + 1}</span><input id="pname-${i}" data-name-idx="${i}" value="${esc(draft.names[i] || '')}" placeholder="Player ${i + 1}"></div>`);
    }
    return `<header><h1>Swiss Rounds</h1><span class="muted">Magic: The Gathering tournament organiser</span></header>
    <main>
      <div class="card">
        <h2>New tournament</h2>
        <div class="row end">
          <label style="flex:2;min-width:240px">Event name<input id="d-name" value="${esc(draft.name)}" placeholder="Friday Night Magic"></label>
          <label>Number of players<input id="d-count" type="number" min="2" max="1024" value="${draft.count}" style="width:120px"></label>
          <label>Swiss rounds<input id="d-rounds" type="number" min="1" max="20" value="${esc(draft.rounds)}" placeholder="auto (${rec})" style="width:120px"></label>
          <label>Top cut<select id="d-topcut">${[0, 2, 4, 8, 16].map((n) => `<option value="${n}" ${n === draft.topCut ? 'selected' : ''}>${n ? 'Top ' + n : 'No cut'}</option>`).join('')}</select></label>
          <label>Round length (min)<input id="d-minutes" type="number" min="1" max="600" value="${draft.minutes}" style="width:120px"></label>
        </div>
      </div>
      <div class="card">
        <div class="row">
          <h2 style="margin:0">Players</h2>
          <span class="spacer"></span>
          <button data-action="togglePaste">${draft.paste ? 'Hide paste box' : 'Paste a list of names'}</button>
          <button data-action="sortDraft" title="Not needed for pairings (round 1 is random) — just tidies the list">Sort A→Z</button>
        </div>
        ${draft.paste ? `<div style="margin-top:12px"><textarea id="d-paste" rows="8" style="width:100%" placeholder="One name per line (or comma separated)"></textarea>
          <div class="row" style="margin-top:8px"><button data-action="usePaste" class="primary">Use these names</button></div></div>` : ''}
        <div class="setup-names">${inputs.join('')}</div>
        <div class="row" style="margin-top:16px">
          <span class="muted" id="d-summary"></span>
          <span class="spacer"></span>
          <button class="primary" data-action="create">Create tournament</button>
        </div>
      </div>
    </main>`;
  }

  // ----- Pairings & results -----
  function renderRound() {
    const t = state.tournament;
    const n = t.rounds.length;
    if (!n) {
      const active = t.players.filter((p) => !p.dropped).length;
      return `<div class="card"><h2>Ready to start</h2>
        <p>${active} players registered · ${t.numRounds} Swiss rounds${t.topCut ? ` · cut to Top ${t.topCut}` : ''} · ${Math.round(t.timer.durationMs / 60000)} minute rounds</p>
        <button class="primary" data-action="pair">Pair Round 1</button></div>`;
    }
    const rn = Math.min(viewRound || n, n);
    const r = t.rounds[rn - 1];
    const latest = rn === n;
    const locked = !!t.playoff;
    const pts = state.roundPoints[rn - 1] || {};
    const reported = r.matches.filter((m) => m.reported).length;
    const complete = reported === r.matches.length;
    const f = filter.trim().toLowerCase();

    const rows = r.matches.map((m, i) => {
      if (f && !(nameOf(m.p1).toLowerCase().includes(f) || (m.p2 && nameOf(m.p2).toLowerCase().includes(f)) || String(m.table) === f)) return '';
      return matchRow(m, i, rn, pts, latest && !locked, locked);
    }).join('');

    let actions = '';
    if (latest && !locked) {
      const more = n < t.numRounds;
      let primary;
      if (!complete) primary = `<span class="muted">Waiting on ${r.matches.length - reported} result${r.matches.length - reported === 1 ? '' : 's'}…</span>`;
      else if (more) primary = `<button class="primary" data-action="pair">Pair Round ${n + 1}</button>`;
      else if (t.topCut) primary = `<button class="primary" data-action="startPlayoff">Cut to Top ${t.topCut}</button>`;
      else primary = `<span class="badge green">Swiss complete — see final standings</span>`;
      const inRound = [];
      for (const m of r.matches) { inRound.push(m.p1); if (m.p2) inRound.push(m.p2); }
      inRound.sort((a, b) => nameOf(a).localeCompare(nameOf(b)));
      const opts = inRound.map((id) => `<option value="${id}">${esc(nameOf(id))}</option>`).join('');
      actions = `<div class="card"><div class="row">
        ${primary}
        <span class="spacer"></span>
        <span class="muted" style="font-size:13px">Swap seats:</span>
        <select id="swapA">${opts}</select><select id="swapB">${opts}</select>
        <button data-action="swap">Swap</button>
        <button class="danger" data-action="deleteRound" title="Delete this round's pairings and results so it can be re-paired">Delete Round ${n}</button>
      </div></div>`;
    }

    return `<div class="card">
      <div class="row">
        <h2 style="margin:0">Round ${rn}</h2>
        <span class="badge ${complete ? 'green' : 'gold'}">${reported}/${r.matches.length} results</span>
        <div class="seg">${t.rounds.map((x) => `<button class="small ${x.number === rn ? 'active' : ''}" data-action="viewRound" data-round="${x.number}">R${x.number}</button>`).join('')}</div>
        <span class="spacer"></span>
        <input id="filter" placeholder="Find player or table…" value="${esc(filter)}" style="width:220px">
      </div>
      ${locked ? '<p class="muted">The top cut has started — Swiss results are locked. Cancel the top cut to edit them.</p>' : ''}
      ${!latest && !locked ? '<p class="muted">Editing an earlier round. Standings and tiebreakers update automatically.</p>' : ''}
    </div>
    ${actions}
    <div class="card" style="padding:0">
      <table class="data matches">
        <thead><tr><th class="c">Table</th><th>Player 1</th><th class="c">Result (player 1 – player 2 – draws)</th><th class="r">Player 2</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="4" class="muted c">No matches</td></tr>'}</tbody>
      </table>
    </div>`;
  }

  function playerCell(id, pts, cls, showDrop, align) {
    const p = state.tournament.players.find((x) => x.id === id);
    let drop = '';
    if (showDrop && p) {
      drop = p.dropped
        ? `<span class="badge red">dropped</span><span class="drop-link" data-action="drop" data-id="${id}" data-dropped="false">undo</span>`
        : `<span class="drop-link" data-action="drop" data-id="${id}" data-dropped="true">drop</span>`;
    }
    const name = `${esc(p ? p.name : '?')} <span class="pts">(${pts[id] ?? 0})</span>`;
    return `<td class="player ${align} ${cls}">${align === 'p2' ? drop + ' ' + name : name + ' ' + drop}</td>`;
  }

  function matchRow(m, i, rn, pts, current, locked) {
    if (!m.p2) {
      return `<tr class="done"><td class="tbl">—</td>${playerCell(m.p1, pts, 'win', current, 'p1')}
        <td class="res"><span class="score">2-0</span> <span class="muted">(bye)</span></td><td class="player p2 muted">BYE</td></tr>`;
    }
    const w = winner(m);
    const c1 = m.reported ? (w === m.p1 ? 'win' : w ? 'lose' : '') : '';
    const c2 = m.reported ? (w === m.p2 ? 'win' : w ? 'lose' : '') : '';
    const key = `${rn}:${i}`;
    let res;
    if (locked) {
      res = m.reported ? `<span class="score">${score(m)}</span>` : '<span class="muted">—</span>';
    } else if (customOpen === key) {
      res = `<span class="res-buttons res-custom">
        <input id="c-w1" type="number" min="0" max="3" value="${m.p1Wins}" title="Player 1 game wins">–
        <input id="c-w2" type="number" min="0" max="3" value="${m.p2Wins}" title="Player 2 game wins">–
        <input id="c-d" type="number" min="0" max="5" value="${m.draws}" title="Drawn games">
        <button class="primary" data-action="customSave" data-round="${rn}" data-match="${i}">Save</button>
        <button data-action="customCancel">Cancel</button></span>`;
    } else {
      const btns = QUICK.map((q) => {
        if (!q) return '<span class="sep"></span>';
        const on = m.reported && m.p1Wins === q[0] && m.p2Wins === q[1] && m.draws === q[2];
        return `<button class="${on ? 'active' : ''}" title="${q[4] || q[3]}" data-action="result" data-stage="swiss" data-round="${rn}" data-match="${i}" data-r="${q[0]},${q[1]},${q[2]}">${q[3]}</button>`;
      }).join('');
      const isQuick = QUICK.some((q) => q && m.p1Wins === q[0] && m.p2Wins === q[1] && m.draws === q[2]);
      res = `<span class="res-buttons">${btns}
        <span class="sep"></span>
        <button class="${m.reported && !isQuick ? 'active' : ''}" title="Custom result" data-action="custom" data-key="${key}">${m.reported && !isQuick ? score(m) : '⋯'}</button>
        <button class="danger" title="Clear result" data-action="clear" data-stage="swiss" data-round="${rn}" data-match="${i}" ${m.reported ? '' : 'style="visibility:hidden"'}>✕</button>
      </span>`;
    }
    return `<tr class="${m.reported ? 'done' : ''}"><td class="tbl">${m.table}</td>
      ${playerCell(m.p1, pts, c1, current, 'p1')}<td class="res">${res}</td>${playerCell(m.p2, pts, c2, current, 'p2')}</tr>`;
  }

  // ----- Standings -----
  function renderStandings() {
    const t = state.tournament;
    const st = state.standings;
    const showFinish = !!t.playoff;
    let activeCount = 0;
    const rows = st.map((s) => {
      let cut = '';
      if (!t.playoff && t.topCut && !s.dropped) {
        activeCount++;
        if (activeCount === t.topCut) cut = ' style="border-bottom:2px solid var(--gold)"';
      }
      return `<tr class="${s.dropped ? 'dropped' : ''}"${cut}>
        <td class="r num">${s.rank}</td>
        <td>${esc(s.name)} ${s.dropped ? '<span class="badge red">dropped</span>' : ''}</td>
        ${showFinish ? `<td>${s.finish ? `<span class="badge gold">${esc(s.finish)}</span>` : ''}</td>` : ''}
        <td class="r num"><b>${s.points}</b></td>
        <td class="c num">${record(s)}</td>
        <td class="r num">${pct(s.omw)}</td>
        <td class="r num">${pct(s.gwp)}</td>
        <td class="r num">${pct(s.ogw)}</td>
      </tr>`;
    }).join('');
    const title = state.standingsRound ? `Standings after Round ${state.standingsRound}` : 'Standings (no completed rounds yet)';
    return `<div class="card" style="padding:0">
      <div class="row" style="padding:16px 20px">
        <h2 style="margin:0">${title}</h2>
        <span class="spacer"></span>
        <a href="/api/standings.csv"><button>Export CSV</button></a>
      </div>
      <table class="data">
        <thead><tr><th class="r">#</th><th>Player</th>${showFinish ? '<th>Finish</th>' : ''}<th class="r">Points</th><th class="c">W-L-D</th><th class="r">OMW%</th><th class="r">GW%</th><th class="r">OGW%</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
      <p class="muted" style="padding:0 20px 16px;font-size:13px">
        Ranked by match points (win 3, draw 1), then Opponents' Match-Win %, Game-Win %, and Opponents' Game-Win %.
        Percentages have a 33.33% floor. Byes count as a 2-0 match win and are ignored for opponent averages.
        Only fully completed rounds are counted.${t.topCut && !t.playoff ? ` The gold line marks the Top ${t.topCut} cut.` : ''}
      </p>
    </div>`;
  }

  // ----- Players -----
  function renderPlayers() {
    const t = state.tournament;
    const byId = new Map(state.standings.map((s) => [s.playerId, s]));
    const rows = [...t.players].sort((a, b) => a.name.localeCompare(b.name)).map((p) => {
      const s = byId.get(p.id);
      return `<tr class="${p.dropped ? 'dropped' : ''}">
        <td><input id="rn-${p.id}" value="${esc(p.name)}" style="width:100%"></td>
        <td class="r num">${s ? s.points : 0}</td>
        <td class="c num">${s ? record(s) : '0-0-0'}</td>
        <td>${p.dropped ? '<span class="badge red">dropped</span>' : '<span class="badge green">active</span>'}</td>
        <td class="r">
          <button class="small" data-action="rename" data-id="${p.id}">Save name</button>
          ${p.dropped
            ? `<button class="small" data-action="drop" data-id="${p.id}" data-dropped="false">Re-enter</button>`
            : `<button class="small danger" data-action="drop" data-id="${p.id}" data-dropped="true">Drop</button>`}
        </td></tr>`;
    }).join('');
    return `<div class="card">
      <h2>Add a player</h2>
      <div class="row">
        <input id="newPlayer" placeholder="Player name" style="width:280px" ${t.playoff ? 'disabled' : ''}>
        <button class="primary" data-action="addPlayer" ${t.playoff ? 'disabled' : ''}>Add player</button>
        <span class="muted" style="font-size:13px">Late entries start with 0 points and are paired from the next round.</span>
      </div>
    </div>
    <div class="card" style="padding:0">
      <table class="data">
        <thead><tr><th>Name</th><th class="r">Points</th><th class="c">W-L-D</th><th>Status</th><th></th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
      <p class="muted" style="padding:0 20px 16px;font-size:13px">Dropped players are not paired in future rounds but still count towards their opponents' tiebreakers.</p>
    </div>`;
  }

  // ----- Top cut -----
  function renderTopCut() {
    const t = state.tournament;
    if (!t.playoff) {
      const seeds = state.standings.filter((s) => !s.dropped).slice(0, t.topCut);
      const cur = t.rounds[t.rounds.length - 1];
      const ready = cur && cur.matches.every((m) => m.reported);
      const remaining = t.numRounds - t.rounds.length;
      return `<div class="card">
        <h2>Top ${t.topCut}</h2>
        <p>${remaining > 0 ? `${remaining} Swiss round${remaining === 1 ? '' : 's'} remaining. ` : 'Swiss rounds complete. '}
        The cut takes the top ${t.topCut} non-dropped players from the standings. Higher seeds are listed first and choose to play or draw.</p>
        <button class="primary" data-action="startPlayoff" ${ready ? '' : 'disabled'}>Cut to Top ${t.topCut}</button>
        ${ready ? '' : '<span class="muted" style="margin-left:8px">Finish reporting the current round first.</span>'}
      </div>
      <div class="card"><h3>${state.standingsRound ? `Projected seeds (after Round ${state.standingsRound})` : 'Projected seeds'}</h3>
        <ol>${seeds.map((s) => `<li>${esc(s.name)} <span class="muted">(${s.points} pts)</span></li>`).join('')}</ol>
      </div>`;
    }
    return `<div class="card">
      <div class="row"><h2 style="margin:0">Top ${t.playoff.size}</h2><span class="spacer"></span>
      <button class="danger" data-action="cancelPlayoff">Cancel top cut</button></div>
      <p class="muted" style="font-size:13px">Click the score next to the player who won the match. Higher seed is listed first.</p>
      ${renderBracket(true)}
    </div>`;
  }

  function renderBracket(editable) {
    const p = state.tournament.playoff;
    const seedOf = (id) => p.seeds.indexOf(id) + 1;
    const cols = state.playoffRoundNames.map((name, ri) => {
      const r = p.rounds[ri];
      const count = p.size >> (ri + 1);
      const matches = [];
      for (let i = 0; i < count; i++) {
        const m = r && r.matches[i];
        matches.push(bracketMatch(m, ri + 1, i, seedOf, editable));
      }
      return `<div class="bround"><h3>${esc(name)}</h3><div class="bmatches">${matches.join('')}</div></div>`;
    });
    const champ = state.champion
      ? `<div class="bround"><h3>Champion</h3><div class="bmatches"><div class="champion-box">🏆 ${esc(nameOf(state.champion))}</div></div></div>`
      : '';
    return `<div class="bracket">${cols.join('')}${champ}</div>`;
  }

  function bracketMatch(m, round, idx, seedOf, editable) {
    if (!m) {
      return `<div class="bmatch"><div class="bp"><span class="seed"></span><span class="nm muted">TBD</span></div><div class="bp"><span class="seed"></span><span class="nm muted">TBD</span></div></div>`;
    }
    const w = winner(m);
    const line = (id, wins, isP1) => {
      const cls = w ? (w === id ? 'win' : 'lose') : '';
      const btns = editable ? (isP1
        ? `<button class="small ${m.reported && m.p1Wins === 2 && m.p2Wins === 0 ? 'active' : ''}" data-action="result" data-stage="playoff" data-round="${round}" data-match="${idx}" data-r="2,0,0">2-0</button>
           <button class="small ${m.reported && m.p1Wins === 2 && m.p2Wins === 1 ? 'active' : ''}" data-action="result" data-stage="playoff" data-round="${round}" data-match="${idx}" data-r="2,1,0">2-1</button>`
        : `<button class="small ${m.reported && m.p2Wins === 2 && m.p1Wins === 0 ? 'active' : ''}" data-action="result" data-stage="playoff" data-round="${round}" data-match="${idx}" data-r="0,2,0">2-0</button>
           <button class="small ${m.reported && m.p2Wins === 2 && m.p1Wins === 1 ? 'active' : ''}" data-action="result" data-stage="playoff" data-round="${round}" data-match="${idx}" data-r="1,2,0">2-1</button>`) : '';
      return `<div class="bp ${cls}"><span class="seed">${seedOf(id)}</span><span class="nm">${esc(nameOf(id))}</span>
        ${m.reported ? `<span class="gw">${wins}</span>` : ''}${btns}</div>`;
    };
    const ctl = editable && m.reported
      ? `<div class="bctl row"><span class="muted" style="font-size:12px">Table ${m.table} · ${score(m)}</span><span class="spacer"></span><button class="small danger" data-action="clear" data-stage="playoff" data-round="${round}" data-match="${idx}">Clear</button></div>`
      : editable ? `<div class="bctl"><span class="muted" style="font-size:12px">Table ${m.table}</span></div>` : '';
    return `<div class="bmatch">${line(m.p1, m.p1Wins, true)}${line(m.p2, m.p2Wins, false)}${ctl}</div>`;
  }

  // ----- Settings -----
  function renderSettings() {
    const t = state.tournament;
    const active = t.players.filter((p) => !p.dropped).length;
    return `<div class="card">
      <h2>Event settings</h2>
      <div class="row end">
        <label style="flex:1;min-width:240px">Event name<input id="s-name" value="${esc(t.name)}"></label>
        <label>Swiss rounds <span>(recommended ${SR.recommendedRounds(active)})</span><input id="s-rounds" type="number" min="${Math.max(1, t.rounds.length)}" max="20" value="${t.numRounds}" style="width:120px"></label>
        <label>Top cut<select id="s-topcut" ${t.playoff ? 'disabled' : ''}>${[0, 2, 4, 8, 16].map((n) => `<option value="${n}" ${n === t.topCut ? 'selected' : ''}>${n ? 'Top ' + n : 'No cut'}</option>`).join('')}</select></label>
        <label>Round length (min)<input id="s-minutes" type="number" min="1" max="600" value="${Math.round(t.timer.durationMs / 60000)}" style="width:120px"></label>
        <button class="primary" data-action="saveSettings">Save</button>
      </div>
      <p class="muted" style="font-size:13px">Changing the round length resets the round timer.</p>
    </div>
    <div class="card">
      <h2>New tournament</h2>
      <p class="muted">Everything is saved automatically. Starting a new tournament archives the current one as a <code>.bak</code> file next to the data file.</p>
      <button class="danger" data-action="newTournament">Start a new tournament</button>
    </div>`;
  }

  // ---------- Actions ----------
  const num = (v) => parseInt(v, 10) || 0;
  const actions = {
    tab(b) { tab = b.dataset.tab; localStorage.setItem('sr.tab', tab); customOpen = null; render(); },
    viewRound(b) { viewRound = num(b.dataset.round); customOpen = null; render(); },
    async pair() {
      await api('POST', '/api/rounds');
      viewRound = null;
      filter = '';
      if (state.tournament.display !== 'pairings') await api('PATCH', '/api/tournament', { display: 'pairings' });
      toast(`Round ${state.tournament.rounds.length} paired — start the timer when players are seated`);
    },
    result(b) {
      const [p1Wins, p2Wins, draws] = b.dataset.r.split(',').map(Number);
      customOpen = null;
      return api('POST', '/api/results', { stage: b.dataset.stage, round: num(b.dataset.round), match: num(b.dataset.match), p1Wins, p2Wins, draws });
    },
    clear(b) {
      return api('POST', '/api/results', { stage: b.dataset.stage, round: num(b.dataset.round), match: num(b.dataset.match), clear: true });
    },
    custom(b) { customOpen = b.dataset.key; render(); $('#c-w1')?.focus(); },
    customCancel() { customOpen = null; render(); },
    async customSave(b) {
      await api('POST', '/api/results', {
        stage: 'swiss', round: num(b.dataset.round), match: num(b.dataset.match),
        p1Wins: num($('#c-w1').value), p2Wins: num($('#c-w2').value), draws: num($('#c-d').value),
      });
      customOpen = null;
      render();
    },
    drop(b) {
      const dropped = b.dataset.dropped === 'true';
      const id = num(b.dataset.id);
      if (dropped && !confirm(`Drop ${nameOf(id)} from the tournament? They won't be paired in future rounds.`)) return;
      return api('PATCH', `/api/players/${id}`, { dropped });
    },
    rename(b) { return api('PATCH', `/api/players/${b.dataset.id}`, { name: $(`#rn-${b.dataset.id}`).value }).then(() => toast('Name saved')); },
    async addPlayer() {
      const name = $('#newPlayer').value.trim();
      if (!name) return;
      await api('POST', '/api/players', { name });
      toast(`${name} added`);
      $('#newPlayer')?.focus();
    },
    deleteRound() {
      const n = state.tournament.rounds.length;
      if (!confirm(`Delete Round ${n}? All of its pairings and results will be removed so you can re-pair it.`)) return;
      viewRound = null;
      return api('DELETE', '/api/rounds/last');
    },
    swap() {
      const a = num($('#swapA').value), b = num($('#swapB').value);
      return api('POST', '/api/rounds/swap', { a, b }).then(() => toast(`Swapped ${nameOf(a)} and ${nameOf(b)}`));
    },
    async startPlayoff() {
      const t = state.tournament;
      const remaining = t.numRounds - t.rounds.length;
      const msg = remaining > 0
        ? `There ${remaining === 1 ? 'is' : 'are'} still ${remaining} Swiss round${remaining === 1 ? '' : 's'} scheduled. Cut to Top ${t.topCut} now anyway?`
        : `Cut to Top ${t.topCut}? Swiss results will be locked.`;
      if (!confirm(msg)) return;
      await api('POST', '/api/playoff');
      tab = 'topcut';
      localStorage.setItem('sr.tab', tab);
      await api('PATCH', '/api/tournament', { display: 'bracket' });
    },
    cancelPlayoff() {
      if (!confirm('Cancel the top cut? All top cut results will be lost and Swiss results unlocked.')) return;
      return api('DELETE', '/api/playoff');
    },
    timer(b) { return api('POST', '/api/timer', { action: b.dataset.op, seconds: num(b.dataset.sec) }); },
    display(b) { return api('PATCH', '/api/tournament', { display: b.dataset.mode }); },
    announce() { return api('PATCH', '/api/tournament', { announcement: $('#announce').value.trim() }); },
    clearAnnounce() { return api('PATCH', '/api/tournament', { announcement: '' }); },
    async saveSettings() {
      const t = state.tournament;
      const body = { name: $('#s-name').value.trim(), numRounds: num($('#s-rounds').value), topCut: num($('#s-topcut').value) };
      const mins = num($('#s-minutes').value);
      if (mins !== Math.round(t.timer.durationMs / 60000)) {
        if (t.timer.running && !confirm('Changing the round length will reset the running timer. Continue?')) return;
        body.roundMinutes = mins;
      }
      await api('PATCH', '/api/tournament', body);
      toast('Settings saved');
    },
    newTournament() {
      if (!confirm('Start a new tournament? The current one will be archived and cleared from the screen.')) return;
      if (!confirm('Are you sure? This cannot be undone from the app.')) return;
      viewRound = null;
      return api('DELETE', '/api/tournament');
    },
    // Setup
    togglePaste() { draft.paste = !draft.paste; render(); },
    usePaste() {
      const names = $('#d-paste').value.split(/[\n,]/).map((s) => s.trim()).filter(Boolean);
      if (!names.length) return toast('No names found', true);
      draft.names = names;
      draft.count = names.length;
      draft.paste = false;
      render();
    },
    sortDraft() {
      draft.names = draft.names.slice(0, draft.count).filter((s) => s && s.trim()).sort((a, b) => a.localeCompare(b));
      render();
    },
    async create() {
      const names = draft.names.slice(0, draft.count).map((s) => (s || '').trim());
      const missing = names.findIndex((s) => !s);
      if (missing !== -1 || names.length < draft.count) {
        toast(`Please enter a name for player ${missing === -1 ? names.length + 1 : missing + 1}`, true);
        document.getElementById(`pname-${missing === -1 ? names.length : missing}`)?.focus();
        return;
      }
      const lower = names.map((s) => s.toLowerCase());
      const dup = lower.findIndex((s, i) => lower.indexOf(s) !== i);
      if (dup !== -1) return toast(`Duplicate player name: ${names[dup]}`, true);
      await api('POST', '/api/tournament', {
        name: draft.name, players: names, numRounds: num(draft.rounds),
        topCut: draft.topCut, roundMinutes: num(draft.minutes),
      });
      tab = 'round';
      toast('Tournament created');
    },
  };

  document.addEventListener('click', async (e) => {
    const b = e.target.closest('[data-action]');
    if (!b || b.disabled) return;
    try { await actions[b.dataset.action](b); } catch (err) { console.error(err); }
  });

  document.addEventListener('input', (e) => {
    const el = e.target;
    if (el.id === 'filter') { filter = el.value; render(); return; }
    if (el.dataset.nameIdx !== undefined) { draft.names[num(el.dataset.nameIdx)] = el.value; updateSummary(); return; }
    switch (el.id) {
      case 'd-name': draft.name = el.value; break;
      case 'd-rounds': draft.rounds = el.value; break;
      case 'd-minutes': draft.minutes = el.value; break;
      case 'd-count': {
        const n = num(el.value);
        if (n >= 2 && n <= 1024) { draft.count = n; render(); }
        break;
      }
    }
  });

  document.addEventListener('change', (e) => {
    if (e.target.id === 'd-topcut') draft.topCut = num(e.target.value);
  });

  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Enter') return;
    const id = e.target.id || '';
    if (id === 'newPlayer') actions.addPlayer();
    else if (id === 'announce') actions.announce();
    else if (id.startsWith('rn-')) actions.rename({ dataset: { id: id.slice(3) } });
    else if (id.startsWith('c-')) $('[data-action="customSave"]')?.click();
    else if (id.startsWith('pname-')) document.getElementById(`pname-${num(id.slice(6)) + 1}`)?.focus();
  });

  function updateSummary() {
    const el = $('#d-summary');
    if (!el) return;
    const filled = draft.names.slice(0, draft.count).filter((s) => s && s.trim()).length;
    el.textContent = `${filled} of ${draft.count} names entered · ${draft.rounds || SR.recommendedRounds(draft.count)} Swiss rounds`;
  }

  async function load() {
    const t0 = Date.now();
    const res = await fetch('/api/state');
    const data = await res.json();
    offset = data.serverNow - (t0 + Date.now()) / 2;
    state = data;
    render();
    updateSummary();
  }

  setInterval(tick, 250);
  window.addEventListener('focus', () => load().catch(() => {}));
  load().catch((e) => { console.error(e); toast('Cannot load tournament: ' + e.message, true); });
})();
