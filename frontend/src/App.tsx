import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom';
import VinInput from './components/VinInput';
import OemSearch from './components/OemSearch';
import Catalog from './components/Catalog';

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="bg-white border-b border-gray-200 px-6 py-3">
        <div className="max-w-5xl mx-auto flex items-center gap-8">
          <span className="text-lg font-bold text-gray-900">Parts Engine</span>
          <NavLink
            to="/"
            end
            className={({ isActive }) =>
              `text-sm font-medium ${isActive ? 'text-blue-600' : 'text-gray-500 hover:text-gray-700'}`
            }
          >
            VIN Decode
          </NavLink>
          <NavLink
            to="/oem"
            className={({ isActive }) =>
              `text-sm font-medium ${isActive ? 'text-blue-600' : 'text-gray-500 hover:text-gray-700'}`
            }
          >
            Smart Search
          </NavLink>
          <NavLink
            to="/catalog"
            className={({ isActive }) =>
              `text-sm font-medium ${isActive ? 'text-blue-600' : 'text-gray-500 hover:text-gray-700'}`
            }
          >
            Catalog
          </NavLink>
        </div>
      </nav>
      <main className="max-w-7xl mx-auto px-6 py-8">{children}</main>
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
    </BrowserRouter>
  );
}
