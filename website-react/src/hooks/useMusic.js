import { useEffect, useRef, useState } from 'react'

const MUSIC_SRC = '/assets/audio/b8083359_audio.m4a'
const DEFAULT_VOLUME = 50
const MAX_VOLUME = 0.9

export function useMusic() {
  const [playing, setPlaying] = useState(false)
  const [volume, setVolume] = useState(DEFAULT_VOLUME)
  const audioRef = useRef(null)

  function getAudio() {
    if (!audioRef.current) {
      const audio = new Audio(MUSIC_SRC)
      audio.loop = true
      audio.preload = 'auto'
      audio.volume = (volume / 100) * MAX_VOLUME
      audioRef.current = audio
    }
    return audioRef.current
  }

  function toggle() {
    const audio = getAudio()
    if (playing) {
      pause()
      return
    }

    audio.play()
      .then(() => setPlaying(true))
      .catch(() => setPlaying(false))
  }

  function pause() {
    if (!audioRef.current) return
    audioRef.current.pause()
    setPlaying(false)
  }

  useEffect(() => {
    if (audioRef.current) {
      audioRef.current.volume = (volume / 100) * MAX_VOLUME
    }
  }, [volume])

  useEffect(() => () => {
    if (!audioRef.current) return
    audioRef.current.pause()
    audioRef.current.src = ''
    audioRef.current = null
  }, [])

  useEffect(() => {
    window.addEventListener('site:pause-music', pause)
    return () => window.removeEventListener('site:pause-music', pause)
  }, [])

  return { playing, toggle, volume, setVolume }
}
