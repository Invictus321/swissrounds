// Shared helpers for the organiser and big-screen views.
window.SR = (() => {
  const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  const pct = (x) => (x * 100).toFixed(2);

  function remainingMs(timer, offset) {
    return timer.running ? timer.endsAtMs - (Date.now() + offset) : timer.remainingMs;
  }

  function fmtClock(ms) {
    const neg = ms < 0;
    const total = neg ? Math.floor(-ms / 1000) : Math.ceil(ms / 1000);
    const h = Math.floor(total / 3600);
    const m = Math.floor((total % 3600) / 60);
    const s = String(total % 60).padStart(2, '0');
    const body = h ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`;
    return (neg ? '−' : '') + body;
  }

  function clockClass(ms) {
    if (ms <= 0) return 'over';
    if (ms < 5 * 60 * 1000) return 'warn';
    return '';
  }

  function players(t) {
    const map = new Map();
    for (const p of t.players) map.set(p.id, p);
    return map;
  }

  function winner(m) {
    if (!m.reported) return 0;
    if (!m.p2) return m.p1;
    if (m.p1Wins > m.p2Wins) return m.p1;
    if (m.p2Wins > m.p1Wins) return m.p2;
    return 0;
  }

  function score(m) {
    return `${m.p1Wins}-${m.p2Wins}${m.draws ? '-' + m.draws : ''}`;
  }

  function record(s) {
    return `${s.wins}-${s.losses}-${s.draws}`;
  }

  function stageLabel(state) {
    const t = state.tournament;
    if (!t) return '';
    if (t.playoff) {
      if (state.champion) return `Top ${t.playoff.size} · Complete`;
      return `Top ${t.playoff.size} · ${state.playoffRoundNames[t.playoff.rounds.length - 1]}`;
    }
    if (!t.rounds.length) return `Registration · ${t.players.filter((p) => !p.dropped).length} players`;
    if (state.phase === 'complete') return `Swiss complete · ${t.rounds.length} rounds`;
    return `Round ${t.rounds.length} of ${t.numRounds}`;
  }

  // Wizards' recommended Swiss rounds for a player count.
  function recommendedRounds(n) {
    const caps = [[1, 0], [2, 1], [4, 2], [8, 3], [16, 4], [32, 5], [64, 6], [128, 7], [226, 8], [409, 9]];
    for (const [cap, r] of caps) if (n <= cap) return r;
    return 10;
  }

  return { esc, pct, remainingMs, fmtClock, clockClass, players, winner, score, record, stageLabel, recommendedRounds };
})();
