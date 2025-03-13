import React, { FC } from 'react';
import { Trans } from 'react-i18next';

const Agent: FC = () => {
  return (
    <>
      <h2>
        <Trans>Prometheus Agent</Trans>
      </h2>
      <p>
        <Trans>
          This Prometheus instance is running in <strong>agent mode</strong>. In this mode, Prometheus is only used to scrape
          discovered targets and forward the scraped metrics to remote write endpoints.
        </Trans>
      </p>
      <p>
        <Trans>Some features are not available in this mode, such as querying and alerting.</Trans>
      </p>
    </>
  );
};

export default Agent;
