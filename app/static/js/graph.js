class ForceGraph {
  constructor(container) {
    this.container = container;
    this.cy = null;
    this.onSelect = null;
    this._layout = 'breadthfirst';
    this._groupIds = [];
  }

  load(data) {
    if (this.cy) this.cy.destroy();

    const elements = [];
    this._groupIds = [];

    data.nodes.forEach(n => {
      elements.push({ data: { id: String(n.id), label: n.label, type: n.type, ip: n.ip || '' } });
      if (n.type === 'group') this._groupIds.push('#' + n.id);
    });

    data.links.forEach(l => {
      elements.push({ data: { id: 'e-' + l.source + '-' + l.target, source: String(l.source), target: String(l.target) } });
    });

    this.cy = cytoscape({
      container: this.container,
      elements,
      style: [
        {
          selector: 'node',
          style: {
            'label': 'data(label)',
            'color': '#8b949e',
            'font-size': '11px',
            'font-family': 'system-ui, sans-serif',
            'text-valign': 'bottom',
            'text-margin-y': '5px',
            'text-max-width': '80px',
            'min-zoomed-font-size': '8px',
          },
        },
        {
          selector: 'node[type="host"]',
          style: {
            'background-color': '#58a6ff',
            'shape': 'ellipse',
            'width': '28px',
            'height': '28px',
          },
        },
        {
          selector: 'node[type="group"]',
          style: {
            'background-color': '#a371f7',
            'shape': 'round-rectangle',
            'width': '38px',
            'height': '38px',
          },
        },
        {
          selector: 'edge',
          style: {
            'width': 1.5,
            'line-color': '#30363d',
            'target-arrow-color': '#4d5561',
            'target-arrow-shape': 'triangle',
            'arrow-scale': 0.8,
            'curve-style': 'bezier',
          },
        },
        {
          selector: 'node.highlighted',
          style: {
            'border-width': '2.5px',
            'border-color': '#ffffff',
            'opacity': 1,
          },
        },
        {
          selector: 'node.dimmed',
          style: { 'opacity': 0.15 },
        },
        {
          selector: 'edge.highlighted',
          style: {
            'line-color': '#58a6ff',
            'target-arrow-color': '#58a6ff',
            'opacity': 1,
          },
        },
        {
          selector: 'edge.dimmed',
          style: { 'opacity': 0.05 },
        },
      ],
      layout: this._layoutConfig(),
      userZoomingEnabled: true,
      userPanningEnabled: true,
      boxSelectionEnabled: false,
      minZoom: 0.1,
      maxZoom: 6,
      wheelSensitivity: 0.3,
    });

    this.cy.on('tap', 'node', (evt) => {
      const node = evt.target;
      const neighborhood = node.closedNeighborhood();
      this.cy.elements().addClass('dimmed').removeClass('highlighted');
      neighborhood.removeClass('dimmed').addClass('highlighted');
      neighborhood.edges().addClass('highlighted').removeClass('dimmed');

      if (this.onSelect) {
        const connectedNodes = node.neighborhood().nodes().map(n => ({
          id: n.id(),
          label: n.data('label'),
          type: n.data('type'),
          ip: n.data('ip'),
        }));
        this.onSelect({
          id: node.id(),
          label: node.data('label'),
          type: node.data('type'),
          ip: node.data('ip'),
        }, connectedNodes);
      }
    });

    this.cy.on('tap', (evt) => {
      if (evt.target === this.cy) {
        this.cy.elements().removeClass('dimmed highlighted');
      }
    });
  }

  _layoutConfig() {
    if (this._layout === 'breadthfirst') {
      return {
        name: 'breadthfirst',
        directed: true,
        roots: this._groupIds.length ? this._groupIds : undefined,
        padding: 48,
        spacingFactor: 1.75,
        avoidOverlap: true,
      };
    }
    return {
      name: 'cose',
      animate: false,
      nodeRepulsion: () => 6000,
      gravity: 0.6,
      idealEdgeLength: () => 120,
      padding: 48,
    };
  }

  setLayout(name) {
    this._layout = name;
    if (this.cy) {
      this.cy.layout(this._layoutConfig()).run();
    }
  }

  fit() {
    if (this.cy) this.cy.fit(undefined, 48);
  }

  resetView() {
    if (this.cy) this.cy.reset();
  }
}
