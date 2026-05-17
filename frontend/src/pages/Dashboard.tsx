import type { ReactNode } from 'react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '../api'
import { getTimeRangeISO, getBucketConfig, type TimeRangeKey } from '../lib/timeRange'
import PageHeader from '../components/PageHeader'
import StateShell from '../components/StateShell'
import StatCard from '../components/StatCard'
import UsageStatsSummary from '../components/UsageStatsSummary'
import type { StatsResponse, UsageStats, ChartAggregation, AccountUsageSummaryRow } from '../types'
import { useDataLoader } from '../hooks/useDataLoader'
import { Card, CardContent } from '@/components/ui/card'
import { Users, CheckCircle, XCircle, Activity } from 'lucide-react'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'

const DashboardUsageCharts = lazy(() => import('../components/DashboardUsageCharts'))

const DASHBOARD_REFRESH_INTERVAL_MS = 15_000

function ChartsSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      {[0, 1, 2, 3].map((i) => (
        <Card key={i} className="py-0">
          <CardContent className="p-6">
            <div className="mb-5 space-y-2">
              <div className="h-4 w-32 rounded-md bg-muted animate-pulse" />
              <div className="h-3 w-48 rounded-md bg-muted/60 animate-pulse" />
            </div>
            <div className="h-[280px] flex items-end gap-2 px-4 pb-4">
              {[40, 65, 30, 80, 55, 70, 45, 60, 35, 75, 50, 68].map((h, j) => (
                <div
                  key={j}
                  className="flex-1 rounded-t-md bg-muted/50 animate-pulse"
                  style={{ height: `${h}%`, animationDelay: `${j * 80}ms` }}
                />
              ))}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

export default function Dashboard() {
  const { t } = useTranslation()
  const [timeRange, setTimeRange] = useState<TimeRangeKey>('1h')
  const [chartData, setChartData] = useState<ChartAggregation | null>(null)
  const [chartRefreshedAt, setChartRefreshedAt] = useState<number | null>(null)
  const [chartLoading, setChartLoading] = useState(true)
  const chartAbort = useRef<AbortController | null>(null)

  // 仅加载轻量级统计数据（秒级响应）
  const loadDashboardStats = useCallback(async () => {
    const [stats, usageStats] = await Promise.all([
      api.getStats(),
      api.getUsageStats(),
    ])
    return { stats, usageStats }
  }, [])

  const { data, loading, error, reload, reloadSilently } = useDataLoader<{
    stats: StatsResponse | null
    usageStats: UsageStats | null
  }>({
    initialData: { stats: null, usageStats: null },
    load: loadDashboardStats,
  })

  // 加载服务端聚合的图表数据（12~48 个聚合点，非原始行）
  const loadChartData = useCallback(async () => {
    chartAbort.current?.abort()
    const controller = new AbortController()
    chartAbort.current = controller
    setChartLoading(true)
    try {
      const { start, end } = getTimeRangeISO(timeRange)
      const { bucketMinutes } = getBucketConfig(timeRange)
      const res = await api.getChartData({ start, end, bucketMinutes })
      if (!controller.signal.aborted) {
        setChartData(res)
        setChartRefreshedAt(Date.now())
      }
    } catch {
      // 静默容错
    } finally {
      if (!controller.signal.aborted) {
        setChartLoading(false)
      }
    }
  }, [timeRange])

  // 首次加载 + timeRange 变更时重新拉取图表数据
  useEffect(() => {
    void loadChartData()
  }, [loadChartData])

  // 加载账号维度的用量汇总
  const [accountSummary, setAccountSummary] = useState<AccountUsageSummaryRow[]>([])
  const [accountSummaryLoading, setAccountSummaryLoading] = useState(false)
  const accountSummaryAbort = useRef<AbortController | null>(null)

  const loadAccountSummary = useCallback(async () => {
    accountSummaryAbort.current?.abort()
    const controller = new AbortController()
    accountSummaryAbort.current = controller
    setAccountSummaryLoading(true)
    try {
      const { start, end } = getTimeRangeISO(timeRange)
      const res = await api.getAccountUsageSummary({ start, end })
      if (!controller.signal.aborted) {
        setAccountSummary(res.accounts ?? [])
      }
    } catch {
      // 静默容错
    } finally {
      if (!controller.signal.aborted) {
        setAccountSummaryLoading(false)
      }
    }
  }, [timeRange])

  useEffect(() => {
    void loadAccountSummary()
  }, [loadAccountSummary])

  // 仅在 1h（实时）模式下启用自动刷新
  useEffect(() => {
    if (timeRange !== '1h') return

    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return
      void reloadSilently()
      void loadChartData()
      void loadAccountSummary()
    }, DASHBOARD_REFRESH_INTERVAL_MS)

    return () => window.clearInterval(timer)
  }, [reloadSilently, timeRange, loadChartData, loadAccountSummary])

  const { stats, usageStats } = data
  const total = stats?.total ?? 0
  const available = stats?.available ?? 0
  const errorCount = stats?.error ?? 0
  const todayRequests = stats?.today_requests ?? 0

  const icons: Record<string, ReactNode> = {
    total: <Users className="size-[22px]" />,
    available: <CheckCircle className="size-[22px]" />,
    error: <XCircle className="size-[22px]" />,
    requests: <Activity className="size-[22px]" />,
  }

  return (
    <StateShell
      variant="page"
      loading={loading}
      error={error}
      onRetry={() => { void reload(); void loadChartData() }}
      loadingTitle={t('dashboard.loadingTitle')}
      loadingDescription={t('dashboard.loadingDesc')}
      errorTitle={t('dashboard.errorTitle')}
    >
      <>
        <PageHeader
          title={t('dashboard.title')}
          description={t('dashboard.description')}
          onRefresh={() => { void reload(); void loadChartData() }}
        />

        {/* Account status */}
        <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-4 mb-6">
          <StatCard icon={icons.total} iconClass="blue" label={t('dashboard.totalAccounts')} value={total} />
          <StatCard
            icon={icons.available}
            iconClass="green"
            label={t('dashboard.available')}
            value={available}
          />
          <StatCard icon={icons.error} iconClass="red" label={t('dashboard.error')} value={errorCount} />
          <StatCard icon={icons.requests} iconClass="purple" label={t('dashboard.todayRequests')} value={todayRequests} />
        </div>

        {/* Usage stats */}
        {usageStats && (
          <div className="space-y-6">
            <UsageStatsSummary stats={usageStats} />
            <Suspense fallback={<ChartsSkeleton />}>
              <DashboardUsageCharts
                chartData={chartData}
                refreshedAt={chartRefreshedAt}
                refreshIntervalMs={DASHBOARD_REFRESH_INTERVAL_MS}
                timeRange={timeRange}
                onTimeRangeChange={setTimeRange}
                loading={chartLoading}
              />
            </Suspense>

            {/* 账号维度用量汇总 */}
            {accountSummary.length > 0 && (
              <Card className="py-0">
                <CardContent className="p-4 sm:p-5">
                  <h3 className="mb-3 text-base font-semibold text-foreground">
                    {t('dashboard.accountConsumption')}
                  </h3>
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="text-[13px]">{t('dashboard.account')}</TableHead>
                          <TableHead className="text-[13px] text-right">{t('dashboard.requestsLabel')}</TableHead>
                          <TableHead className="text-[13px] text-right">{t('dashboard.inputTokensLabel')}</TableHead>
                          <TableHead className="text-[13px] text-right">{t('dashboard.outputTokensLabel')}</TableHead>
                          <TableHead className="text-[13px] text-right">{t('dashboard.totalTokensLabel')}</TableHead>
                          <TableHead className="text-[13px] text-right">{t('dashboard.costLabel')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {accountSummary.map((row) => (
                          <TableRow key={row.account_id}>
                            <TableCell className="text-[13px] font-medium max-w-[200px] truncate" title={row.email || `#${row.account_id}`}>
                              {row.email || `#${row.account_id}`}
                            </TableCell>
                            <TableCell className="text-[13px] text-right tabular-nums">{row.requests.toLocaleString()}</TableCell>
                            <TableCell className="text-[13px] text-right tabular-nums">{row.input_tokens.toLocaleString()}</TableCell>
                            <TableCell className="text-[13px] text-right tabular-nums">{row.output_tokens.toLocaleString()}</TableCell>
                            <TableCell className="text-[13px] text-right tabular-nums">{row.total_tokens.toLocaleString()}</TableCell>
                            <TableCell className="text-[13px] text-right tabular-nums font-semibold text-emerald-600 dark:text-emerald-400">
                              {formatDashCost(row.user_billed || row.account_billed)}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        )}
      </>
    </StateShell>
  )
}

function formatDashCost(value: number): string {
  if (!value || value <= 0) return '$0.00'
  if (value >= 100) return `${value.toLocaleString(undefined, { maximumFractionDigits: 1 })}`
  if (value >= 1) return `${value.toFixed(2)}`
  return `${value.toFixed(4)}`
}