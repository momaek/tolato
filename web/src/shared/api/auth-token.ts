const STORAGE_KEY = "tolato.session.token"

let authToken = readStoredToken()

export function getAuthToken() {
  return authToken
}

export function hasAuthToken() {
  return authToken.length > 0
}

export function setAuthToken(token: string) {
  authToken = token.trim()
  if (typeof window !== "undefined") {
    if (authToken) {
      window.localStorage.setItem(STORAGE_KEY, authToken)
    } else {
      window.localStorage.removeItem(STORAGE_KEY)
    }
  }
}

export function clearAuthToken() {
  setAuthToken("")
}

function readStoredToken() {
  if (typeof window === "undefined") {
    return ""
  }

  return window.localStorage.getItem(STORAGE_KEY) ?? ""
}
