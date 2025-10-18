# Human vs AI Gameplay Architecture (PLAYING_CLAUDE.md)

## Vision

Enable human players to compete against trained AI agents in any game supported by the Cartridge platform. The system should be as modular as the Engine service - adding a new playable game should require minimal code and no changes to core services.

## Design Principles

### 1. **Cartridge Philosophy**
- **Plug-and-Play**: New games drop in like cartridges into a console
- **Zero Server Changes**: Adding games only requires UI components and metadata
- **Generic Interfaces**: Core services remain game-agnostic
- **Consistent Experience**: Same interaction patterns across all games

### 2. **Separation of Concerns**
- **Engine Service**: Game simulation and rules (unchanged)
- **Web Service**: Human interface and session management
- **Game UI Components**: Game-specific rendering and input handling
- **AI Opponent Service**: Model loading and AI move generation
- **Session Orchestration**: Game state management and player coordination

### 3. **Modularity Requirements**
- Game UIs as self-contained components
- Declarative game metadata (capabilities, UI hints, assets)
- Hot-swappable game frontends
- Standardized game lifecycle hooks

## Architecture Overview

```
Human Player ←→ Web Browser ←→ Web Service ←→ Game Session Manager
                                      ↓
                               Engine Service (game logic)
                                      ↓
                               AI Opponent Service ←→ Weights Service
```

## Component Responsibilities

### Game Session Manager (New)
**Purpose**: Orchestrates human vs AI gameplay sessions

**Responsibilities**:
- Manage game state and turn order
- Coordinate between human input and AI moves
- Handle game lifecycle (start, pause, forfeit, complete)
- Maintain session persistence and recovery
- Emit events for spectators/recordings

**Key Interfaces**:
- `CreateSession(game_id, human_player, ai_config) → session_id`
- `MakeMove(session_id, player_id, action) → updated_state`
- `GetGameState(session_id) → current_state`
- `SubscribeToSession(session_id) → event_stream`

### AI Opponent Service (New)
**Purpose**: Provides AI players for human gameplay

**Responsibilities**:
- Load trained models from Weights service
- Generate AI moves given game state
- Support multiple AI difficulty levels
- Handle model versioning and A/B testing
- Provide AI personality/behavior configuration

**Key Interfaces**:
- `LoadModel(game_id, model_version, difficulty) → ai_player_id`
- `GenerateMove(ai_player_id, game_state, legal_actions) → action`
- `GetAvailableAIs(game_id) → [ai_configurations]`

### Web Service (Enhanced)
**Purpose**: Serves human players and manages UI

**Enhancements**:
- Game discovery and selection interface
- Real-time gameplay UI rendering
- Human input capture and validation
- Session management (join, leave, spectate)
- Player profiles and game history

### Game UI Registry (New)
**Purpose**: Manages pluggable game interfaces

**Structure**:
```
games/
├── metadata/
│   ├── tictactoe.json     # Game capabilities and UI hints
│   ├── chess.json
│   └── poker.json
├── components/
│   ├── tictactoe/         # Self-contained UI component
│   │   ├── board.html
│   │   ├── styles.css
│   │   ├── game.js
│   │   └── assets/
│   ├── chess/
│   └── poker/
└── shared/
    ├── base.css           # Common styling
    ├── game-framework.js  # Standard game hooks
    └── ui-components.js   # Reusable UI elements
```

## Game Metadata Schema

Each game provides a metadata file describing its UI requirements:

```json
{
  "game_id": "tictactoe",
  "display_name": "Tic-Tac-Toe",
  "description": "Classic 3x3 grid game",
  "min_players": 2,
  "max_players": 2,
  "supports_ai": true,
  "estimated_duration": "2-5 minutes",

  "ui": {
    "component_path": "tictactoe/",
    "viewport": "square",
    "input_methods": ["click", "touch"],
    "supports_spectating": true,
    "requires_real_time": false
  },

  "ai": {
    "difficulty_levels": ["beginner", "intermediate", "expert"],
    "default_model": "tictactoe-v1.2",
    "supports_hints": true,
    "max_thinking_time": "1s"
  },

  "engine": {
    "env_id": "tictactoe",
    "min_engine_version": "0.1.0",
    "action_encoding": "discrete_position:v1",
    "state_display_format": "board_view"
  }
}
```

## Data Flow Patterns

### Game Initialization
1. Human selects game from discovery interface
2. Web service queries Engine for game capabilities
3. Web service loads game UI component and metadata
4. Session Manager creates new session with Engine
5. AI Opponent Service loads appropriate model
6. Game state streams to browser

### Human Move Flow
1. Human interacts with game UI (click, drag, etc.)
2. UI validates move locally (immediate feedback)
3. Web service sends move to Session Manager
4. Session Manager validates with Engine service
5. Engine returns new state and game status
6. Session Manager triggers AI move if needed
7. Updated state streams to all session participants

### AI Move Flow
1. Session Manager detects AI turn
2. Requests move from AI Opponent Service
3. AI service generates move using loaded model
4. Move sent to Engine via Session Manager
5. New state propagated to human player

## Key Abstractions

### Game Component Interface
Every game UI implements a standard interface:

```javascript
class GameComponent {
  // Lifecycle
  initialize(gameState, playerConfig)
  destroy()

  // State Management
  updateState(newState)
  getLocalState()

  // Player Interaction
  onPlayerMove(callback)
  highlightLegalMoves(actions)
  showGameResult(winner, reason)

  // AI Integration
  showAIThinking(isThinking)
  displayMoveHint(suggestedMove)

  // Spectator Mode
  enableSpectatorMode()
  showMoveHistory(moves)
}
```

### Session Events
Standardized events for game lifecycle:

```
SessionCreated { session_id, players, game_config }
PlayerJoined { session_id, player_id, player_info }
MoveAttempted { session_id, player_id, action, timestamp }
MoveCompleted { session_id, action, new_state, next_player }
GameCompleted { session_id, winner, reason, final_state }
PlayerDisconnected { session_id, player_id }
```

## Scalability Considerations

### Horizontal Scaling
- Session Manager can be stateless with external state store
- Multiple AI Opponent Service instances behind load balancer
- Game UI components served from CDN
- WebSocket connections distributed across web instances

### Performance Optimization
- Game state deltas instead of full state updates
- Client-side move validation for immediate feedback
- AI move caching for repeated positions
- Preloaded game assets and UI components

### Resource Management
- Session timeout and cleanup policies
- AI model pooling and lifecycle management
- Connection limits per game session
- Background session persistence

## Adding New Games - Developer Experience

### Step 1: Engine Integration
Game already implemented in Engine service (existing process)

### Step 2: UI Component Creation
```bash
# Generate game scaffold
./scripts/new-game-ui.sh chess

# Creates:
games/components/chess/
├── game.js          # Game component implementation
├── board.html       # Game-specific HTML templates
├── styles.css       # Game-specific styling
└── assets/          # Images, sounds, etc.
```

### Step 3: Metadata Configuration
Create `games/metadata/chess.json` with game specifications

### Step 4: AI Configuration (Optional)
Define available AI models and difficulty levels

### Step 5: Testing & Deployment
- Component unit tests
- Integration tests with Engine service
- UI responsiveness testing
- Deploy to game registry

## Advanced Features

### Spectator Mode
- Real-time viewing of ongoing games
- Move annotations and analysis
- Chat integration for spectators
- Replay saved games

### Tournament Mode
- Bracket-style tournaments
- Swiss system pairing
- Rating and leaderboard integration
- Automated scheduling

### AI Personalities
- Multiple AI models per game with different styles
- Named AI opponents with consistent behavior
- Difficulty that adapts to human skill level
- AI that explains its reasoning

### Analytics & Learning
- Game telemetry collection
- Human move pattern analysis
- AI performance evaluation
- A/B testing of different AI models

## Migration Strategy

### Phase 1: Foundation
- Implement Session Manager and AI Opponent Service
- Create Game UI Registry infrastructure
- Build first game component (TicTacToe)

### Phase 2: Core Features
- Add real-time session management
- Implement spectator mode
- Create developer tooling for new games

### Phase 3: Advanced Features
- Tournament and matchmaking systems
- Advanced AI personalities
- Mobile-responsive game interfaces

### Phase 4: Ecosystem
- Third-party game component support
- Plugin marketplace
- Advanced analytics and insights

## Success Metrics

- **Developer Experience**: Time to add new playable game < 1 day
- **Performance**: < 100ms move response time, support 1000+ concurrent sessions
- **User Experience**: Consistent UI patterns, mobile-friendly, sub-second AI moves
- **Reliability**: 99.9% session completion rate, graceful handling of disconnections