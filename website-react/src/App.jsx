import { lazy, Suspense } from 'react'
import { Routes, Route } from 'react-router-dom'
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
const Game = lazy(() => import('./pages/Game'))

export default function App() {
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
        <Route path="game" element={<Suspense fallback={null}><Game /></Suspense>} />
        <Route path="*" element={<Home />} />
      </Route>
    </Routes>
  )
}
