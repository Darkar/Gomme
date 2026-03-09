package inventory

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// ADConfig est la configuration runtime de la source ActiveDirectory (mot de passe en clair).
type ADConfig struct {
	URL      string
	BindDN   string
	Password string
	Insecure bool
	Groups   []ADGroupConfig
}

// serverType représente le type de serveur LDAP détecté.
type serverType int

const (
	serverAD      serverType = iota // Active Directory (Windows)
	serverOpenLDAP                  // OpenLDAP ou annuaire LDAP générique
)

// ADSource implémente Source pour un annuaire ActiveDirectory ou OpenLDAP.
// Le type de serveur est détecté automatiquement via le rootDSE.
// Chaque groupe correspond à une recherche LDAP (filtre + base DN).
type ADSource struct {
	Config ADConfig
}

// detectServerType interroge le rootDSE pour déterminer si le serveur est AD ou OpenLDAP.
// Présence de "domainFunctionality" → Active Directory.
func detectServerType(conn *ldap.Conn) serverType {
	req := ldap.NewSearchRequest(
		"",
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(objectClass=*)",
		[]string{"domainFunctionality"},
		nil,
	)
	res, err := conn.Search(req)
	if err != nil || len(res.Entries) == 0 {
		return serverOpenLDAP
	}
	if res.Entries[0].GetAttributeValue("domainFunctionality") != "" {
		return serverAD
	}
	return serverOpenLDAP
}

func (a *ADSource) Sync() ([]HostData, []GroupData, error) {
	conn, err := a.dial()
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	if err := conn.Bind(a.Config.BindDN, a.Config.Password); err != nil {
		return nil, nil, fmt.Errorf("authentification LDAP (%s): %w", a.Config.BindDN, err)
	}

	srvType := detectServerType(conn)

	// hostSet évite les doublons (un même CN peut apparaître dans plusieurs groupes).
	hostSet := map[string]*HostData{}
	var groupDatas []GroupData

	for _, grp := range a.Config.Groups {
		objClass := "computer"
		if srvType == serverOpenLDAP {
			objClass = "device"
		}
		filter := buildFilter(objClass, grp.Filter)

		// On demande tous les attributs potentiels ; ceux absents du schéma
		// de l'entrée seront simplement ignorés par le serveur.
		attrs := []string{"cn", "dNSHostName", "description", "sAMAccountName", "ipHostNumber"}

		req := ldap.NewSearchRequest(
			grp.BaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			filter,
			attrs,
			nil,
		)

		res, err := conn.Search(req)
		if err != nil {
			return nil, nil, fmt.Errorf("recherche LDAP groupe %q (base: %s): %w", grp.Name, grp.BaseDN, err)
		}

		groupDatas = append(groupDatas, GroupData{Name: grp.Name, Description: "AD/LDAP"})

		for _, entry := range res.Entries {
			cn := entry.GetAttributeValue("cn")
			if cn == "" {
				continue
			}
			// Résolution de l'adresse/hostname dans l'ordre de priorité :
			// dNSHostName (AD computer) → ipHostNumber (ipHost) → sAMAccountName (AD fallback) → vide
			ip := entry.GetAttributeValue("dNSHostName")
			if ip == "" {
				ip = entry.GetAttributeValue("ipHostNumber")
			}
			if ip == "" {
				ip = entry.GetAttributeValue("sAMAccountName")
			}
			if hd, exists := hostSet[cn]; exists {
				hd.Groups = append(hd.Groups, grp.Name)
			} else {
				hostSet[cn] = &HostData{
					Name:        cn,
					IP:          ip,
					Description: firstVal(entry.GetAttributeValues("description")),
					Groups:      []string{grp.Name},
				}
			}
		}
	}

	hosts := make([]HostData, 0, len(hostSet))
	for _, hd := range hostSet {
		hosts = append(hosts, *hd)
	}

	return hosts, groupDatas, nil
}

// dial ouvre la connexion LDAP (ldap:// ou ldaps://).
func (a *ADSource) dial() (*ldap.Conn, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: a.Config.Insecure} //nolint:gosec — option explicite
	url := a.Config.URL

	if strings.HasPrefix(url, "ldaps://") {
		conn, err := ldap.DialURL(url, ldap.DialWithTLSConfig(tlsCfg))
		if err != nil {
			return nil, fmt.Errorf("connexion LDAPS à %s: %w", url, err)
		}
		return conn, nil
	}

	conn, err := ldap.DialURL(url)
	if err != nil {
		return nil, fmt.Errorf("connexion LDAP à %s: %w", url, err)
	}
	return conn, nil
}

// buildFilter construit le filtre LDAP final en imposant objectClass=<objClass>.
// Si l'utilisateur a fourni un filtre additionnel, il est combiné en AND.
func buildFilter(objClass, userFilter string) string {
	base := "(objectClass=" + objClass + ")"
	userFilter = strings.TrimSpace(userFilter)
	if userFilter == "" {
		return base
	}
	if !strings.HasPrefix(userFilter, "(") {
		userFilter = "(" + userFilter + ")"
	}
	return "(&" + base + userFilter + ")"
}

func firstVal(vals []string) string {
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}
