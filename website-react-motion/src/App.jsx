import { lazy, Suspense, useEffect } from 'react'
import { Routes, Route, useLocation } from 'react-router-dom'
import { trackSiteVisit } from './api/analytics'
import Layout from './components/Layout'
import Home from './pages/Home'

// 首页直载（最快首屏）；其余路由懒加载，减小首页 bundle
const Teacher = lazy(() => import('./pages/Teacher'))
const Stages = lazy(() => import('./pages/Stages'))
const Stage1 = lazy(() => import('./pages/Stage1'))
const Stage2 = lazy(() => import('./pages/Stage2'))
const Stage3 = lazy(() => import('./pages/Stage3'))
const Watch = lazy(() => import('./pages/Watch'))
const Course = lazy(() => import('./pages/Course'))
const Courses = lazy(() => import('./pages/Courses'))
const Game = lazy(() => import('./pages/Game'))
const TypeDetail = lazy(() => import('./pages/TypeDetail'))
const Quotes = lazy(() => import('./pages/Quotes'))
const MindQuotes = lazy(() => import('./pages/MindQuotes'))
const MindQuoteDetail = lazy(() => import('./pages/MindQuoteDetail'))
const Types = lazy(() => import('./pages/Types'))
const Signup = lazy(() => import('./pages/Signup'))

const routeFallback = <div className="route-loading" role="status" aria-live="polite">加载中…</div>

function lazyRoute(element) {
  return <Suspense fallback={routeFallback}>{element}</Suspense>
}

export default function App() {
  const location = useLocation()

  useEffect(() => {
    trackSiteVisit()
  }, [location.pathname, location.search, location.hash])

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Home />} />
        <Route path="teacher" element={lazyRoute(<Teacher />)} />
        <Route path="stages" element={lazyRoute(<Stages />)} />
        <Route path="stage1" element={lazyRoute(<Stage1 />)} />
        <Route path="stage2" element={lazyRoute(<Stage2 />)} />
        <Route path="stage3" element={lazyRoute(<Stage3 />)} />
        <Route path="watch" element={lazyRoute(<Watch />)} />
        <Route path="course" element={lazyRoute(<Course />)} />
        <Route path="courses" element={lazyRoute(<Courses />)} />
        <Route path="game" element={lazyRoute(<Game />)} />
        <Route path="type/:id" element={lazyRoute(<TypeDetail />)} />
        <Route path="quotes" element={lazyRoute(<Quotes />)} />
        <Route path="mind-quotes" element={lazyRoute(<MindQuotes />)} />
        <Route path="mind-quotes/:id" element={lazyRoute(<MindQuoteDetail />)} />
        <Route path="types" element={lazyRoute(<Types />)} />
        <Route path="signup" element={lazyRoute(<Signup />)} />
        <Route path="*" element={<Home />} />
      </Route>
    </Routes>
  )
}
