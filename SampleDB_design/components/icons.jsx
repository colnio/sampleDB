// Shared icons — all inline SVG, stroke=currentColor
const Ic = {
  Flask: (p={}) => (
    <svg {...p} viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <path d="M12 4 L12 12 L5 26 Q4 28 6 29 L26 29 Q28 28 27 26 L20 12 L20 4 Z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round"/>
      <line x1="10" y1="4" x2="22" y2="4" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
      <path d="M8.5 20 L23.5 20 L27 26 Q28 28 26 29 L6 29 Q4 28 5 26 Z" fill="currentColor" opacity="0.2"/>
      <circle cx="12" cy="24" r="1.3" fill="currentColor"/>
      <circle cx="16" cy="22" r="1.3" fill="currentColor"/>
      <circle cx="20" cy="25" r="1.3" fill="currentColor"/>
    </svg>
  ),
  Dashboard: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><rect x="3.5" y="3.5" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.5"/><rect x="13.5" y="3.5" width="7" height="4" rx="1.5" stroke="currentColor" strokeWidth="1.5"/><rect x="13.5" y="10.5" width="7" height="10" rx="1.5" stroke="currentColor" strokeWidth="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5" stroke="currentColor" strokeWidth="1.5"/></svg>),
  Samples: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M4 6h16M4 12h16M4 18h10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Wiki: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><rect x="4.5" y="5" width="8" height="14" rx="1.75" stroke="currentColor" strokeWidth="1.5"/><path d="M12.5 5.75H17a1.75 1.75 0 011.75 1.75v11.75" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/><path d="M7.25 9h3.25m-3.25 3h2.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Plus: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M12 5v14M5 12h14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Calendar: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><rect x="3.75" y="5.5" width="16.5" height="14.5" rx="2" stroke="currentColor" strokeWidth="1.5"/><path d="M8 3.75v3.5m8-3.5v3.5M3.75 10.5h16.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/><circle cx="16" cy="15.5" r="1.3" fill="currentColor"/></svg>),
  Key: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><circle cx="7.5" cy="15.5" r="4" stroke="currentColor" strokeWidth="1.5"/><path d="M11 12.5L20.5 3M16.5 7.5l2 2M19 5l2 2" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Shield: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M12 3l7 3v6c0 4.23-2.86 8.09-7 9-4.14-.91-7-4.77-7-9V6z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/><path d="M12 11.5a2 2 0 100 4 2 2 0 000-4z" fill="currentColor"/></svg>),
  Search: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><circle cx="11" cy="11" r="6" stroke="currentColor" strokeWidth="1.5"/><path d="M20 20l-4-4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Moon: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M20 14.5A8 8 0 119.5 4a7 7 0 0010.5 10.5z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" fill="currentColor" fillOpacity="0.15"/></svg>),
  Sun: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="4" stroke="currentColor" strokeWidth="1.5"/><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4L7 17M17 7l1.4-1.4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Grid: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><rect x="4" y="4" width="7" height="7" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="13" y="4" width="7" height="7" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="4" y="13" width="7" height="7" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="13" y="13" width="7" height="7" rx="1" stroke="currentColor" strokeWidth="1.5"/></svg>),
  List: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M4 6h16M4 12h16M4 18h16" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Kanban: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><rect x="4" y="4" width="5" height="16" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="10" y="4" width="5" height="10" rx="1" stroke="currentColor" strokeWidth="1.5"/><rect x="16" y="4" width="4" height="13" rx="1" stroke="currentColor" strokeWidth="1.5"/></svg>),
  Filter: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M4 5h16l-6 8v6l-4-2v-4L4 5z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>),
  Download: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M12 4v12m-4-4l4 4 4-4M4 20h16" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>),
  Edit: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M4 20h4L20 8l-4-4L4 16z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>),
  Trash: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>),
  Drag: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="currentColor"><circle cx="9" cy="6" r="1.3"/><circle cx="15" cy="6" r="1.3"/><circle cx="9" cy="12" r="1.3"/><circle cx="15" cy="12" r="1.3"/><circle cx="9" cy="18" r="1.3"/><circle cx="15" cy="18" r="1.3"/></svg>),
  Caret: (p={}) => (<svg {...p} viewBox="0 0 20 20" fill="currentColor"><path d="M5.23 7.21a.75.75 0 011.06.02L10 10.12l3.71-2.89a.75.75 0 11.92 1.18l-4.18 3.26a.75.75 0 01-.92 0L5.21 8.41a.75.75 0 01.02-1.2z"/></svg>),
  Arrow: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M5 12h14m-6-6l6 6-6 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>),
  ArrowLeft: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M19 12H5m6-6l-6 6 6 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>),
  Clock: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="8" stroke="currentColor" strokeWidth="1.5"/><path d="M12 8v4.5l3 2" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Beaker: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M9 3h6v5l4.5 9a1.5 1.5 0 01-1.35 2.1H5.85A1.5 1.5 0 014.5 17L9 8V3z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>),
  Check: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M5 12l5 5 9-11" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>),
  Close: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M6 6l12 12M18 6L6 18" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/></svg>),
  Upload: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M12 20V8m-4 4l4-4 4 4M4 4h16" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/></svg>),
  Dot: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="3"/></svg>),
  Sparkle: (p={}) => (<svg {...p} viewBox="0 0 24 24" fill="none"><path d="M12 3l2 5 5 2-5 2-2 5-2-5-5-2 5-2 2-5z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round"/></svg>),
};

// Abstract sample thumbnail
function SampleThumb({ seed = 1, palette = 'blue', style }) {
  const s = (seed * 12345) % 1000;
  const rand = (n) => ((s * (n + 1) * 9301 + 49297) % 233280) / 233280;
  const palettes = {
    blue: ['#0c4a6e','#0369a1','#38bdf8','#bae6fd'],
    teal: ['#134e4a','#0e7c66','#5eead4','#ccfbf1'],
    violet: ['#3b0764','#7c3aed','#c4b5fd','#ede9fe'],
    amber: ['#78350f','#d97706','#fbbf24','#fef3c7'],
    emerald:['#064e3b','#047857','#34d399','#d1fae5'],
    rose: ['#881337','#e11d48','#fb7185','#ffe4e6'],
  };
  const P = palettes[palette] || palettes.blue;
  const shapes = Array.from({ length: 22 }, (_, i) => ({
    cx: 10 + rand(i) * 380,
    cy: 10 + rand(i * 3) * 160,
    r:  3 + rand(i * 5) * 34,
    c:  P[Math.floor(rand(i*7)*P.length)],
    o:  0.25 + rand(i * 11) * 0.55,
  }));
  return (
    <svg viewBox="0 0 400 180" preserveAspectRatio="xMidYMid slice" style={{ width: '100%', height: '100%', display: 'block', ...style }}>
      <defs>
        <radialGradient id={`g${seed}-${palette}`} cx="30%" cy="30%" r="90%">
          <stop offset="0%" stopColor={P[2]} stopOpacity="0.6"/>
          <stop offset="100%" stopColor={P[0]} stopOpacity="1"/>
        </radialGradient>
      </defs>
      <rect width="400" height="180" fill={`url(#g${seed}-${palette})`}/>
      {shapes.map((c, i) => (
        <circle key={i} cx={c.cx} cy={c.cy} r={c.r} fill={c.c} opacity={c.o} />
      ))}
    </svg>
  );
}

Object.assign(window, { Ic, SampleThumb });
