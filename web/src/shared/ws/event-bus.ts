export function createEventBus<T>() {
  const listeners = new Set<(payload: T) => void>()

  return {
    emit(payload: T) {
      listeners.forEach(listener => listener(payload))
    },
    on(listener: (payload: T) => void) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
}
