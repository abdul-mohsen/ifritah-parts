import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom';
import VinInput from './components/VinInput';
import OemSearch from './components/OemSearch';
import Catalog from './components/Catalog';
import { DebugOverlay } from './components/DebugOverlay';

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_#1e3a8a_0%,_#0f172a_38%,_#020617_100%)] text-slate-100">
      <header className="border-b border-white/10 bg-slate-950/50 backdrop-blur">
        <div className="mx-auto flex max-w-7xl flex-col gap-6 px-6 py-6 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-3xl">
            <div className="mb-3 inline-flex items-center rounded-full border border-sky-400/20 bg-sky-400/10 px-3 py-1 text-xs font-medium uppercase tracking-[0.24em] text-sky-200">
              Evidence-first Hyundai / Kia parts workflow
            </div>
            <h1 className="text-3xl font-semibold tracking-tight text-white sm:text-4xl">
              Parts Engine
            </h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300 sm:text-base">
              Decode the vehicle with NHTSA-backed data, confirm the exact variant, move into catalog browse,
              and inspect parts with provenance, caution labels, and only the visuals the data can honestly support.
            </p>
          </div>

          <nav className="flex flex-wrap gap-2">
            {[
              { to: '/', label: 'VIN decode' },
              { to: '/oem', label: 'Search' },
              { to: '/catalog', label: 'Catalog' },
            ].map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) =>
                  `rounded-full border px-4 py-2 text-sm font-medium transition-colors ${
                    isActive
                      ? 'border-sky-300 bg-sky-400/15 text-white'
                      : 'border-white/10 bg-white/5 text-slate-300 hover:border-white/20 hover:bg-white/10 hover:text-white'
                  }`
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-8">{children}</main>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<VinInput />} />
          <Route path="/oem" element={<OemSearch />} />
          <Route path="/catalog" element={<Catalog />} />
        </Routes>
      </Layout>
      {/* Dev log overlay — only rendered in dev mode (Vite import.meta.env.DEV).
          Connects to /api/debug/logs SSE endpoint when DEBUG_LOGS=1 is set on the server. */}
      {import.meta.env.DEV && <DebugOverlay />}
    </BrowserRouter>
  );
}
