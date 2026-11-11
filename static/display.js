(() => {
  const { esc, pct, fmtClock, clockClass, remainingMs, winner, record } = SR;
  const $ = (sel) => document.querySelector(sel);
  // ?view=pairings|standings|bracket|timer pins this screen to one view.
  const pinned = new URLSearchParams(location.search).get('view');

  let state = null;
  let offset = 0;
  let lastKey = '';
  let failures = 0;

  async function poll() {
    try {
      const t0 = Date.now();
      const res = await fetch('/api/state', { cache: 'no-store' });
      const data = await res.json();
      offset = data.serverNow - (t0 + Date.now()) / 2;
      state = data;
      failures = 0;
      $('#conn').classList.add('hidden');
      const t = data.tournament;
      const key = JSON.stringify([t && { ...t, timer: null }, data.standings, data.phase]);
      if (key !== lastKey) {
        lastKey = key;
        render();
      }
      tick();
    } catch {
      if (++failures > 2) $('#conn').classList.remove('hidden');
    }
    setTimeout(poll, 1000);
  }

  function mode() {
    return pinned || (state.tournament && state.tournament.display) || 'pairings';
  }

  function nameOf(id) {
    const p = state.tournament.players.find((x) => x.id === id);
    return p ? p.name : '?';
  }

  function stageText() {
    const t = state.tournament;
    if (mode() === 'standings' && !t.playoff) {
      if (state.phase === 'complete') return 'Final Standings';
      return state.standingsRound ? `Standings after Round ${state.standingsRound}` : 'Standings';
    }
    if (mode() === 'standings' && state.champion) return 'Final Standings';
    if (mode() === 'pairings' && t.rounds.length && !t.playoff) return `Round ${t.rounds.length} of ${t.numRounds} · Pairings`;
    return SR.stageLabel(state);
  }

  function render() {
    const t = state.tournament;
    const header = $('#header'), main = $('#main'), footer = $('#footer');
    if (!t) {
      header.innerHTML = '';
      footer.classList.add('hidden');
      main.innerHTML = `<div class="center-msg"><h2>Swiss Rounds</h2><p>Waiting for the tournament to begin…</p></div>`;
      return;
    }
    const m = mode();
    header.innerHTML = `<div class="title"><h1>${esc(t.name)}</h1><div class="stage">${esc(stageText())}</div></div>
      ${m === 'timer' ? '' : '<div><div class="clock js-clock"></div><div class="clock-sub js-clock-sub"></div></div>'}`;
    if (t.announcement) {
      footer.textContent = t.announcement;
      footer.classList.remove('hidden');
    } else {
      footer.classList.add('hidden');
    }
    switch (m) {
      case 'standings': renderStandings(main); break;
      case 'bracket': renderBracket(main); break;
      case 'timer': renderTimer(main); break;
      default: renderPairings(main);
    }
    tick();
  }

  function tick() {
    if (!state || !state.tournament) return;
    const timer = state.tournament.timer;
    const ms = remainingMs(timer, offset);
    for (const el of document.querySelectorAll('.js-clock')) {
      el.textContent = ms <= 0 ? 'TIME' : fmtClock(ms);
      el.className = `${el.classList.contains('big-clock') ? 'big-clock ' : ''}clock js-clock ${clockClass(ms)} ${timer.running ? '' : 'paused'}`;
    }
    for (const el of document.querySelectorAll('.js-clock-sub')) {
      if (ms <= 0) el.textContent = `+${fmtClock(-ms)} · Turn 0 + 5 turns`;
      else el.textContent = timer.running ? '' : 'Paused';
    }
    for (const el of document.querySelectorAll('.js-time-called')) {
      el.classList.toggle('hidden', ms > 0);
    }
  }

  // Renders rows into as many side-by-side tables as gives the largest
  // readable font that fits the screen without scrolling.
  function fitTable(main, head, rows, maxFont = 64) {
    main.innerHTML = '<div class="fit"></div>';
    const fit = main.firstChild;
    const minFont = 10;
    let best = null;
    for (let cols = 1; cols <= 4; cols++) {
      if (cols > 1 && rows.length / cols < 4) break;
      const per = Math.ceil(rows.length / cols);
      let html = '';
      for (let c = 0; c < cols; c++) {
        const chunk = rows.slice(c * per, (c + 1) * per);
        if (chunk.length) html += `<div class="fit-col"><table class="big"><thead>${head}</thead><tbody>${chunk.join('')}</tbody></table></div>`;
      }
      fit.innerHTML = html;
      let lo = minFont, hi = maxFont;
      if (fits(fit, hi)) lo = hi;
      else {
        while (hi - lo > 1) {
          const mid = (lo + hi) >> 1;
          if (fits(fit, mid)) lo = mid; else hi = mid;
        }
      }
      // Only use more columns if it gives a noticeably bigger font.
      if (!best || lo > best.size * 1.15) best = { size: lo, html };
    }
    fit.innerHTML = best.html;
    fit.style.fontSize = best.size + 'px';
  }

  function fits(fit, size) {
    fit.style.fontSize = size + 'px';
    for (const col of fit.children) {
      if (col.scrollHeight > col.clientHeight + 1 || col.scrollWidth > col.clientWidth + 1) return false;
    }
    return true;
  }

  function centerMsg(main, title, sub) {
    main.innerHTML = `<div class="center-msg"><h2>${esc(title)}</h2>${sub ? `<p>${esc(sub)}</p>` : ''}</div>`;
  }

  function renderPairings(main) {
    const t = state.tournament;
    if (t.playoff) {
      const p = t.playoff;
      const r = p.rounds[p.rounds.length - 1];
      const seed = (id) => p.seeds.indexOf(id) + 1;
      const rows = r.matches.map((m) => {
        const w = winner(m);
        return `<tr class="${m.reported ? 'done' : ''}"><td class="tbl">${m.table}</td>
          <td class="name">${w === m.p1 ? '✓ ' : ''}<span class="pts">(${seed(m.p1)})</span> ${esc(nameOf(m.p1))}</td>
          <td class="c pts">vs</td>
          <td class="name">${w === m.p2 ? '✓ ' : ''}<span class="pts">(${seed(m.p2)})</span> ${esc(nameOf(m.p2))}</td></tr>`;
      });
      fitTable(main, '<tr><th class="c">Table</th><th>Higher seed</th><th></th><th>Lower seed</th></tr>', rows, 72);
      return;
    }
    if (!t.rounds.length) {
      const names = t.players.filter((p) => !p.dropped).map((p) => p.name).sort((a, b) => a.localeCompare(b));
      fitTable(main, `<tr><th>Registered players (${names.length})</th></tr>`, names.map((n) => `<tr><td class="name">${esc(n)}</td></tr>`));
      return;
    }
    const r = t.rounds[t.rounds.length - 1];
    const pts = state.roundPoints[t.rounds.length - 1] || {};
    const entries = [];
    for (const m of r.matches) {
      entries.push({ id: m.p1, opp: m.p2, table: m.table, done: m.reported });
      if (m.p2) entries.push({ id: m.p2, opp: m.p1, table: m.table, done: m.reported });
    }
    entries.sort((a, b) => nameOf(a.id).localeCompare(nameOf(b.id)));
    const rows = entries.map((e) => `<tr class="${e.done && e.opp ? 'done' : ''}">
      <td class="tbl">${e.table || '—'}</td>
      <td class="name">${esc(nameOf(e.id))} <span class="pts">${pts[e.id] ?? 0}</span></td>
      <td>${e.opp ? `${esc(nameOf(e.opp))} <span class="pts">${pts[e.opp] ?? 0}</span>` : '<b>BYE</b>'}</td></tr>`);
    fitTable(main, '<tr><th class="c">Table</th><th>Player <span class="pts">pts</span></th><th>Opponent <span class="pts">pts</span></th></tr>', rows);
  }

  function renderStandings(main) {
    const t = state.tournament;
    if (!state.standingsRound && !t.playoff) {
      centerMsg(main, 'Standings', 'Standings appear once the first round is complete.');
      return;
    }
    const finish = !!t.playoff;
    const rows = state.standings.map((s) => `<tr class="${s.dropped ? 'dropped' : ''}">
      <td class="r">${s.rank}</td>
      <td class="name">${esc(s.name)}${s.dropped ? ' <span class="pts">(drop)</span>' : ''}</td>
      ${finish ? `<td class="finish">${esc(s.finish || '')}</td>` : ''}
      <td class="r"><b>${s.points}</b></td>
      <td class="c">${record(s)}</td>
      <td class="r">${pct(s.omw)}</td>
      <td class="r">${pct(s.gwp)}</td>
      <td class="r">${pct(s.ogw)}</td></tr>`);
    const head = `<tr><th class="r">#</th><th>Player</th>${finish ? '<th></th>' : ''}<th class="r">Pts</th><th class="c">W-L-D</th><th class="r">OMW%</th><th class="r">GW%</th><th class="r">OGW%</th></tr>`;
    fitTable(main, head, rows, 48);
  }

  function renderBracket(main) {
    const t = state.tournament;
    const p = t.playoff;
    if (!p) {
      if (!t.topCut) centerMsg(main, 'No top cut', 'This event is Swiss only.');
      else centerMsg(main, `Top ${t.topCut}`, `The top ${t.topCut} players after ${t.numRounds} rounds of Swiss advance to single elimination.`);
      return;
    }
    const seed = (id) => p.seeds.indexOf(id) + 1;
    const line = (m, id, wins) => {
      const w = winner(m);
      return `<div class="bp ${w ? (w === id ? 'win' : 'lose') : ''}"><span class="seed">${seed(id)}</span><span class="nm">${esc(nameOf(id))}</span>${m.reported ? `<span class="gw">${wins}</span>` : ''}</div>`;
    };
    const tbd = '<div class="bp"><span class="seed"></span><span class="nm muted">TBD</span></div>';
    const cols = state.playoffRoundNames.map((name, ri) => {
      const r = p.rounds[ri];
      const count = p.size >> (ri + 1);
      let html = '';
      for (let i = 0; i < count; i++) {
        const m = r && r.matches[i];
        html += m ? `<div class="bmatch">${line(m, m.p1, m.p1Wins)}${line(m, m.p2, m.p2Wins)}</div>` : `<div class="bmatch">${tbd}${tbd}</div>`;
      }
      return `<div class="bround"><h3>${esc(name)}</h3><div class="bmatches">${html}</div></div>`;
    });
    const champ = `<div class="bround"><h3>Champion</h3><div class="bmatches">${state.champion
      ? `<div class="champion-box"><small>Champion</small>🏆 ${esc(nameOf(state.champion))}</div>`
      : '<div class="champion-box muted" style="border-style:dashed;color:var(--muted)">?</div>'}</div></div>`;
    const fs = { 2: 2.4, 4: 2.2, 8: 1.7, 16: 1.1 }[p.size] || 1.4;
    main.innerHTML = `<div class="dbracket" style="--bfs:${fs}vw">${cols.join('')}${champ}</div>`;
  }

  function renderTimer(main) {
    main.innerHTML = `<div class="center-msg">
      <div class="big-clock-label">${esc(SR.stageLabel(state))}</div>
      <div class="big-clock clock js-clock"></div>
      <div class="time-called js-time-called hidden">Time! Finish the current turn, then 5 additional turns.</div>
      <div class="big-clock-label js-clock-sub"></div>
    </div>`;
  }

  let resizeTimer;
  window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => state && render(), 150);
  });
  setInterval(tick, 200);
  poll();
})();
