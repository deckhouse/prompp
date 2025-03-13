import React, { FC } from 'react';
import { useTranslation } from 'react-i18next';

export interface QueryStats {
  loadTime: number;
  resolution: number;
  resultSeries: number;
}

const QueryStatsView: FC<QueryStats> = (props) => {
  const { loadTime, resolution, resultSeries } = props;
  const { t } = useTranslation('graph');

  return (
    <div className="query-stats">
      <span className="float-right">
        {t('Load time')}: {loadTime}ms &ensp; {t('Resolution')}: {resolution}s &ensp; {t('Result series')}: {resultSeries}
      </span>
    </div>
  );
};

export default QueryStatsView;
