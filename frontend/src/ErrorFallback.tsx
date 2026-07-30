export function ErrorFallback() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-zinc-950 px-4 text-zinc-50">
      <div className="text-center">
        <h1 className="text-2xl font-semibold">Something went wrong</h1>
        <p className="mt-2 text-sm text-zinc-400">
          Please refresh the page. The error has been reported.
        </p>
      </div>
    </main>
  )
}
