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

export default function App() {
  const location = useLocation()

  useEffect(() => {
    trackSiteVisit()
  }, [location.pathname, location.search, location.hash])

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Home />} />
        <Route path="teacher" element={<Suspense fallback={null}><Teacher /></Suspense>} />
        <Route path="stages" element={<Suspense fallback={null}><Stages /></Suspense>} />
        <Route path="stage1" element={<Suspense fallback={null}><Stage1 /></Suspense>} />
        <Route path="stage2" element={<Suspense fallback={null}><Stage2 /></Suspense>} />
        <Route path="stage3" element={<Suspense fallback={null}><Stage3 /></Suspense>} />
        <Route path="watch" element={<Suspense fallback={null}><Watch /></Suspense>} />
        <Route path="course" element={<Suspense fallback={null}><Course /></Suspense>} />
        <Route path="courses" element={<Suspense fallback={null}><Courses /></Suspense>} />
        <Route path="game" element={<Suspense fallback={null}><Game /></Suspense>} />
        <Route path="type/:id" element={<Suspense fallback={null}><TypeDetail /></Suspense>} />
        <Route path="quotes" element={<Suspense fallback={null}><Quotes /></Suspense>} />
        <Route path="mind-quotes" element={<Suspense fallback={null}><MindQuotes /></Suspense>} />
        <Route path="mind-quotes/:id" element={<Suspense fallback={null}><MindQuoteDetail /></Suspense>} />
        <Route path="types" element={<Suspense fallback={null}><Types /></Suspense>} />
        <Route path="signup" element={<Suspense fallback={null}><Signup /></Suspense>} />
        <Route path="*" element={<Home />} />
      </Route>
    </Routes>
  )
}
