// Halaman penanda untuk menu yang fiturnya dibangun di fase berikutnya (lihat Plan/PLAN.md).
export default function Placeholder({ title, phase }: { title: string; phase: string }) {
  return (
    <section>
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="mt-2 text-muted-foreground">Halaman ini dibangun di {phase}.</p>
    </section>
  )
}
