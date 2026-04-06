<script setup lang="ts">
import { computed } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Filler,
  Title,
  Tooltip,
  Legend,
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Title, Tooltip, Legend)

const props = defineProps<{
  title: string
  labels: string[]
  datasets: {
    label: string
    data: (number | null)[]
    borderColor: string
    backgroundColor?: string
    fill?: boolean
  }[]
  yAxisLabel?: string
}>()

const chartData = computed(() => ({
  labels: props.labels,
  datasets: props.datasets.map((ds) => ({
    label: ds.label,
    data: ds.data,
    borderColor: ds.borderColor,
    backgroundColor: ds.backgroundColor || 'transparent',
    fill: ds.fill || false,
    tension: 0.3,
    pointRadius: 1,
    borderWidth: 1.5,
  })),
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: props.datasets.length > 1, labels: { color: '#B8B9B6', font: { size: 10 } } },
    title: { display: true, text: props.title, color: '#FFFFFF', font: { size: 12 } },
  },
  scales: {
    x: { ticks: { color: '#B8B9B6', font: { size: 9 }, maxTicksLimit: 10 }, grid: { color: '#2E2E2E' } },
    y: {
      title: { display: !!props.yAxisLabel, text: props.yAxisLabel || '', color: '#B8B9B6' },
      ticks: { color: '#B8B9B6', font: { size: 9 } },
      grid: { color: '#2E2E2E' },
    },
  },
}))
</script>

<template>
  <div class="rounded-lg p-4" style="background-color: var(--card)">
    <div style="height: 200px">
      <Line :data="chartData" :options="chartOptions" />
    </div>
  </div>
</template>
