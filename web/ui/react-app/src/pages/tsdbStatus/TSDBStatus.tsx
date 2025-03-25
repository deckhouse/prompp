import React, { FC } from 'react';
import { Table } from 'reactstrap';

import { useFetch } from '../../hooks/useFetch';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';
import { useTranslation } from 'react-i18next';

interface Stats {
  name: string;
  value: number;
}

interface HeadStats {
  numSeries: number;
  numLabelPairs: number;
  chunkCount: number;
  minTime: number;
  maxTime: number;
}

export interface TSDBMap {
  headStats: HeadStats;
  seriesCountByMetricName: Stats[];
  labelValueCountByLabelName: Stats[];
  memoryInBytesByLabelName: Stats[];
  seriesCountByLabelValuePair: Stats[];
}

export const TSDBStatusContent: FC<TSDBMap> = ({
  headStats,
  labelValueCountByLabelName,
  seriesCountByMetricName,
  memoryInBytesByLabelName,
  seriesCountByLabelValuePair,
}) => {
  const { t } = useTranslation(['TSDBStatus', 'navigation']);
  const unixToTime = (unix: number): string => {
    try {
      return `${new Date(unix).toISOString()} (${unix})`;
    } catch {
      if (numSeries === 0) {
        return t('No datapoints yet');
      }
      return `${t('Error parsing time')} (${unix})`;
    }
  };
  const { chunkCount, numSeries, numLabelPairs, minTime, maxTime } = headStats;
  const stats = [
    { header: t('Number of Series'), value: numSeries },
    { header: t('Number of Chunks'), value: chunkCount },
    { header: t('Number of Label Pairs'), value: numLabelPairs },
    { header: t('Current Min Time'), value: `${unixToTime(minTime)}` },
    { header: t('Current Max Time'), value: `${unixToTime(maxTime)}` },
  ];
  return (
    <div>
      <h2>{t('TSDB Status', { ns: 'navigation' })}</h2>
      <h3 className="p-2">Head Stats</h3>
      <div className="p-2">
        <Table bordered size="sm" striped>
          <thead>
            <tr>
              {stats.map(({ header }) => {
                return <th key={header}>{header}</th>;
              })}
            </tr>
          </thead>
          <tbody>
            <tr>
              {stats.map(({ header, value }) => {
                return <td key={header}>{value}</td>;
              })}
            </tr>
          </tbody>
        </Table>
      </div>
      <h3 className="p-2">{t('Head Cardinality Stats')}</h3>
      {[
        { title: t('Top 10 label names with value count'), stats: labelValueCountByLabelName },
        { title: t('Top 10 series count by metric names'), stats: seriesCountByMetricName },
        { title: t('Top 10 label names with high memory usage'), unit: 'Bytes', stats: memoryInBytesByLabelName },
        { title: t('Top 10 series count by label value pairs'), stats: seriesCountByLabelValuePair },
      ].map(({ title, unit = 'Count', stats }) => {
        return (
          <div className="p-2" key={title}>
            <h3>{title}</h3>
            <Table bordered size="sm" striped>
              <thead>
                <tr>
                  <th>{t('Name')}</th>
                  <th>{unit}</th>
                </tr>
              </thead>
              <tbody>
                {stats.map(({ name, value }) => {
                  return (
                    <tr key={name}>
                      <td>{name}</td>
                      <td>{value}</td>
                    </tr>
                  );
                })}
              </tbody>
            </Table>
          </div>
        );
      })}
    </div>
  );
};
TSDBStatusContent.displayName = 'TSDBStatusContent';

const TSDBStatusContentWithStatusIndicator = withStatusIndicator(TSDBStatusContent);

const TSDBStatus: FC = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<TSDBMap>(`${pathPrefix}/${API_PATH}/status/tsdb`);

  return (
    <TSDBStatusContentWithStatusIndicator
      error={error}
      isLoading={isLoading}
      {...response.data}
      componentTitle="TSDB Status information"
    />
  );
};

export default TSDBStatus;
