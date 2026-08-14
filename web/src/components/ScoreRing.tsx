import { Progress } from 'antd'

const scoreColor = (s: number) => {
  if (s >= 80) return '#52c41a'
  if (s >= 60) return '#faad14'
  return '#f5222d'
}

export default function ScoreRing({ score, size = 120, label }: { score: number; size?: number; label?: string }) {
  return (
    <div style={{ textAlign: 'center' }}>
      <Progress
        type="circle"
        percent={score}
        size={size}
        strokeColor={scoreColor(score)}
        format={(p) => <span style={{ fontSize: size * 0.28, fontWeight: 700 }}>{p}</span>}
      />
      {label && <div style={{ marginTop: 8, color: '#888' }}>{label}</div>}
    </div>
  )
}
