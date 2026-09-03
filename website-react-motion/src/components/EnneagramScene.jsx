import { useEffect, useRef } from 'react'
import {
  AmbientLight,
  BufferAttribute,
  BufferGeometry,
  Group,
  IcosahedronGeometry,
  Line,
  LineBasicMaterial,
  LineLoop,
  Mesh,
  MeshPhysicalMaterial,
  MeshStandardMaterial,
  PerspectiveCamera,
  PointLight,
  Points,
  PointsMaterial,
  Scene,
  SphereGeometry,
  SRGBColorSpace,
  Vector3,
  WebGLRenderer,
} from 'three'
import { useMotionPreferences } from '../motion/useMotionPreferences'

const NODE_COLORS = [0x70c9a4, 0x8cafe8, 0x70c9a4, 0xf06c68, 0x8cafe8, 0x70c9a4, 0x8cafe8, 0xf06c68, 0xf06c68]

function makeCircle(radius, segments = 160) {
  return Array.from({ length: segments }, (_, index) => {
    const angle = index / segments * Math.PI * 2
    return new Vector3(Math.cos(angle) * radius, Math.sin(angle) * radius, 0)
  })
}

export default function EnneagramScene() {
  const mountRef = useRef(null)
  const reducedMotion = useMotionPreferences()

  useEffect(() => {
    const mount = mountRef.current
    if (!mount) return undefined

    const renderer = new WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'high-performance', preserveDrawingBuffer: true })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.75))
    renderer.setClearColor(0x000000, 0)
    renderer.outputColorSpace = SRGBColorSpace
    mount.appendChild(renderer.domElement)

    const scene = new Scene()
    const camera = new PerspectiveCamera(38, 1, .1, 100)
    camera.position.set(0, 0, 7.4)
    const world = new Group()
    world.position.x = 1.38
    scene.add(world)

    const shellMaterial = new MeshPhysicalMaterial({
      color: 0x16362b,
      emissive: 0x0f2a21,
      emissiveIntensity: .7,
      metalness: .18,
      roughness: .22,
      transmission: .22,
      transparent: true,
      opacity: .92,
      wireframe: true,
    })
    const shell = new Mesh(new IcosahedronGeometry(1.55, 3), shellMaterial)
    world.add(shell)

    const inner = new Mesh(
      new IcosahedronGeometry(1.08, 2),
      new MeshStandardMaterial({ color: 0xa7df65, emissive: 0x4d6c2f, emissiveIntensity: .65, roughness: .38, transparent: true, opacity: .18 }),
    )
    world.add(inner)

    const orbitMaterial = new LineBasicMaterial({ color: 0x21463a, transparent: true, opacity: .34 })
    const outerOrbit = new LineLoop(new BufferGeometry().setFromPoints(makeCircle(2.58)), orbitMaterial)
    const middleOrbit = new LineLoop(new BufferGeometry().setFromPoints(makeCircle(2.08)), orbitMaterial.clone())
    middleOrbit.rotation.x = Math.PI * .42
    middleOrbit.rotation.y = Math.PI * .12
    world.add(outerOrbit, middleOrbit)

    const enneagramIndices = [0, 3, 6, 0, 6, 2, 5, 8, 1, 4, 7, 2]
    const enneagramPoints = enneagramIndices.map((index) => {
      const angle = Math.PI / 2 - index * Math.PI * 2 / 9
      return new Vector3(Math.cos(angle) * 2.05, Math.sin(angle) * 2.05, .06)
    })
    const enneagramLine = new Line(
      new BufferGeometry().setFromPoints(enneagramPoints),
      new LineBasicMaterial({ color: 0xd95352, transparent: true, opacity: .36 }),
    )
    world.add(enneagramLine)

    const nodes = new Group()
    for (let index = 0; index < 9; index += 1) {
      const angle = Math.PI / 2 - index * Math.PI * 2 / 9
      const node = new Mesh(
        new SphereGeometry(.115, 24, 24),
        new MeshStandardMaterial({ color: NODE_COLORS[index], emissive: NODE_COLORS[index], emissiveIntensity: .78, roughness: .2 }),
      )
      node.position.set(Math.cos(angle) * 2.05, Math.sin(angle) * 2.05, .12)
      node.userData.baseScale = index % 3 === 0 ? 1.28 : 1
      nodes.add(node)
    }
    world.add(nodes)

    const particleCount = window.innerWidth < 640 ? 650 : 1500
    const particlePositions = new Float32Array(particleCount * 3)
    for (let index = 0; index < particleCount; index += 1) {
      const phi = Math.acos(2 * Math.random() - 1)
      const theta = Math.random() * Math.PI * 2
      const radius = 2.9 + Math.random() * 2.6
      particlePositions[index * 3] = radius * Math.sin(phi) * Math.cos(theta)
      particlePositions[index * 3 + 1] = radius * Math.sin(phi) * Math.sin(theta)
      particlePositions[index * 3 + 2] = radius * Math.cos(phi) * .52
    }
    const particleGeometry = new BufferGeometry()
    particleGeometry.setAttribute('position', new BufferAttribute(particlePositions, 3))
    const particles = new Points(
      particleGeometry,
      new PointsMaterial({ color: 0x286b51, size: .018, transparent: true, opacity: .55, sizeAttenuation: true }),
    )
    world.add(particles)

    const keyLight = new PointLight(0xb9ff72, 7, 15)
    keyLight.position.set(2, 2.2, 4)
    const fillLight = new PointLight(0x7199d6, 5, 14)
    fillLight.position.set(-3, -1.5, 3)
    scene.add(keyLight, fillLight, new AmbientLight(0xffffff, 1.1))

    let pointerX = 0
    let pointerY = 0
    let frame = 0
    const onPointerMove = (event) => {
      pointerX = event.clientX / window.innerWidth * 2 - 1
      pointerY = event.clientY / window.innerHeight * 2 - 1
    }
    const resize = () => {
      const rect = mount.getBoundingClientRect()
      renderer.setSize(rect.width, rect.height, false)
      camera.aspect = rect.width / Math.max(rect.height, 1)
      camera.updateProjectionMatrix()
      world.position.x = rect.width < 760 ? .15 : 1.38
      world.position.y = rect.width < 760 ? .72 : 0
      world.scale.setScalar(rect.width < 440 ? .72 : rect.width < 760 ? .84 : 1)
    }
    const startedAt = performance.now()
    const draw = (timestamp = startedAt) => {
      const time = (timestamp - startedAt) / 1000
      const scroll = window.scrollY / Math.max(window.innerHeight, 1)
      world.rotation.y += ((pointerX * .18 + scroll * .12) - world.rotation.y) * .035
      world.rotation.x += ((pointerY * .1 + scroll * .05) - world.rotation.x) * .035
      shell.rotation.x = time * .09
      shell.rotation.y = time * .12
      inner.rotation.x = -time * .08
      inner.rotation.z = time * .1
      particles.rotation.z = time * .008
      outerOrbit.rotation.z = time * .025
      nodes.children.forEach((node, index) => {
        const pulse = 1 + Math.sin(time * 1.7 + index) * .18
        node.scale.setScalar(node.userData.baseScale * pulse)
      })
      renderer.render(scene, camera)
      if (!reducedMotion) frame = requestAnimationFrame(draw)
    }

    resize()
    draw()
    window.addEventListener('resize', resize)
    window.addEventListener('pointermove', onPointerMove, { passive: true })
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('resize', resize)
      window.removeEventListener('pointermove', onPointerMove)
      renderer.dispose()
      shell.geometry.dispose()
      shellMaterial.dispose()
      inner.geometry.dispose()
      inner.material.dispose()
      outerOrbit.geometry.dispose()
      outerOrbit.material.dispose()
      middleOrbit.geometry.dispose()
      middleOrbit.material.dispose()
      enneagramLine.geometry.dispose()
      enneagramLine.material.dispose()
      nodes.children.forEach((node) => { node.geometry.dispose(); node.material.dispose() })
      particleGeometry.dispose()
      particles.material.dispose()
      mount.replaceChildren()
    }
  }, [reducedMotion])

  return <div className="enneagram-scene" ref={mountRef} data-webgl-scene aria-hidden="true" />
}
