import React, { FC, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Collapse,
  Navbar,
  NavbarToggler,
  Nav,
  NavItem,
  NavLink,
  UncontrolledDropdown,
  DropdownToggle,
  DropdownMenu,
  DropdownItem,
} from 'reactstrap';
import { ThemeToggle } from './Theme';
import { ReactComponent as PromLogo } from './images/prompp_logo_line.svg';
import { LanguageToggle } from './Language';

interface NavbarProps {
  consolesLink: string | null;
  agentMode: boolean;
  animateLogo?: boolean | false;
}

const Navigation: FC<NavbarProps> = ({ consolesLink, agentMode, animateLogo }) => {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const toggle = () => setIsOpen(!isOpen);
  return (
    <Navbar className="mb-3" dark color="dark" expand="md" fixed="top">
      <NavbarToggler onClick={toggle} className="mr-2" />
      <Link className="pt-0 navbar-brand" to={agentMode ? '/agent' : '/graph'}>
        <PromLogo className={`d-inline-block align-top`} title="Prom++" />
      </Link>
      <Collapse isOpen={isOpen} navbar style={{ justifyContent: 'space-between' }}>
        <Nav className="ml-0" navbar>
          {consolesLink !== null && (
            <NavItem>
              <NavLink href={consolesLink}>{t('Consoles')}</NavLink>
            </NavItem>
          )}
          {!agentMode && (
            <>
              <NavItem>
                <NavLink tag={Link} to="/alerts">
                  {t('Alerts')}
                </NavLink>
              </NavItem>
              <NavItem>
                <NavLink tag={Link} to="/graph">
                  {t('Graph')}
                </NavLink>
              </NavItem>
            </>
          )}
          <UncontrolledDropdown nav inNavbar>
            <DropdownToggle nav caret>
              {t('Status')}
            </DropdownToggle>
            <DropdownMenu>
              <DropdownItem tag={Link} to="/status">
                {t('Runtime & Build Information')}
              </DropdownItem>
              {!agentMode && (
                <DropdownItem tag={Link} to="/tsdb-status">
                  {t('TSDB Status')}
                </DropdownItem>
              )}
              <DropdownItem tag={Link} to="/flags">
                {t('Command-Line Flags')}
              </DropdownItem>
              <DropdownItem tag={Link} to="/config">
                {t('Configuration')}
              </DropdownItem>
              {!agentMode && (
                <DropdownItem tag={Link} to="/rules">
                  {t('Rules')}
                </DropdownItem>
              )}
              <DropdownItem tag={Link} to="/targets">
                {t('Targets')}
              </DropdownItem>
              <DropdownItem tag={Link} to="/service-discovery">
                {t('Service Discovery')}
              </DropdownItem>
            </DropdownMenu>
          </UncontrolledDropdown>
          <NavItem>
            <NavLink href="https://prometheus.io/docs/prometheus/latest/getting_started/">{t('Help')}</NavLink>
          </NavItem>
        </Nav>
      </Collapse>
      <LanguageToggle />
      <ThemeToggle />
    </Navbar>
  );
};

export default Navigation;
