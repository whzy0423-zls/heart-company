import { Routes, Route } from 'react-router-dom'
import ListPage from './pages/ListPage.jsx'
import ReaderPage from './pages/ReaderPage.jsx'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<ListPage />} />
      <Route path="/article/:id" element={<ReaderPage />} />
      <Route path="*" element={<ListPage />} />
    </Routes>
  )
}
